package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

var ErrSessionNotFound = errors.New("session not found")

// SessionManager owns one whatsmeow client per Chatwoot inbox.
type SessionManager struct {
	store   *Store
	cfg     Config
	log     waLog.Logger
	webhook *WebhookSender

	mu       sync.Mutex
	sessions map[string]*Session
}

type Session struct {
	id      string
	rec     SessionRecord
	client  *whatsmeow.Client
	store   *Store
	mgr     *SessionManager
	log     waLog.Logger
	webhook *WebhookSender

	mu        sync.Mutex
	lastQR    string
	pairing   bool
	groupName map[types.JID]string
}

type SessionStatus struct {
	Status string `json:"status"` // connected | disconnected | qr_pending | logged_out
	JID    string `json:"jid,omitempty"`
	Name   string `json:"name,omitempty"`
	QR     string `json:"qr,omitempty"`
}

func NewSessionManager(store *Store, cfg Config, log waLog.Logger) *SessionManager {
	return &SessionManager{store: store, cfg: cfg, log: log, webhook: NewWebhookSender(log.Sub("webhook")),
		sessions: map[string]*Session{}}
}

// RestoreAll loads every registered session; load only connects the paired ones,
// so unpaired sessions stay available for QR pairing after a restart.
func (m *SessionManager) RestoreAll(ctx context.Context) error {
	recs, err := m.store.ListSessions(ctx)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if _, err := m.load(ctx, rec); err != nil {
			m.log.Errorf("restore session %s: %v", rec.ID, err)
		}
	}
	go m.pruneLoop()
	return nil
}

func (m *SessionManager) pruneLoop() {
	retention := time.Duration(max(m.cfg.HistoryDays, 30)) * 24 * time.Hour
	for range time.Tick(6 * time.Hour) {
		if err := m.store.PruneMessages(context.Background(), retention); err != nil {
			m.log.Warnf("prune messages: %v", err)
		}
	}
}

func (m *SessionManager) load(ctx context.Context, rec SessionRecord) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[rec.ID]; ok {
		s.rec = rec
		return s, nil
	}
	device := m.store.Container.NewDevice()
	if rec.DeviceJID != "" {
		jid, err := types.ParseJID(rec.DeviceJID)
		if err != nil {
			return nil, err
		}
		existing, err := m.store.Container.GetDevice(ctx, jid)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			device = existing
		}
	}
	log := m.log.Sub(rec.ID)
	s := &Session{
		id: rec.ID, rec: rec, store: m.store, mgr: m, log: log, webhook: m.webhook,
		client: whatsmeow.NewClient(device, log.Sub("client")), groupName: map[types.JID]string{},
	}
	s.client.AddEventHandler(s.handleEvent)
	m.sessions[rec.ID] = s
	if device.ID != nil {
		if err := s.client.Connect(); err != nil {
			return s, fmt.Errorf("connect: %w", err)
		}
	}
	return s, nil
}

// Create registers (or updates) a session; it does not start pairing.
func (m *SessionManager) Create(ctx context.Context, rec SessionRecord) (*Session, error) {
	if err := m.store.UpsertSession(ctx, rec); err != nil {
		return nil, err
	}
	m.mu.Lock()
	existing, ok := m.sessions[rec.ID]
	m.mu.Unlock()
	if ok {
		existing.mu.Lock()
		existing.rec = rec
		existing.mu.Unlock()
		return existing, nil
	}
	return m.load(ctx, rec)
}

func (m *SessionManager) Get(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return s, nil
}

func (m *SessionManager) Delete(ctx context.Context, id string) error {
	s, err := m.Get(id)
	if err == nil {
		if s.client.Store.ID != nil {
			if err := s.client.Logout(ctx); err != nil {
				s.log.Warnf("logout: %v", err)
				_ = s.client.Store.Delete(ctx)
			}
		}
		s.client.Disconnect()
		m.mu.Lock()
		delete(m.sessions, id)
		m.mu.Unlock()
	}
	return m.store.DeleteSession(ctx, id)
}

func (m *SessionManager) DisconnectAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		s.client.Disconnect()
	}
}

func (s *Session) Status() SessionStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := SessionStatus{}
	switch {
	case s.client.Store.ID == nil && s.pairing:
		st.Status, st.QR = "qr_pending", s.lastQR
	case s.client.Store.ID == nil:
		st.Status = "logged_out"
	case s.client.IsConnected() && s.client.IsLoggedIn():
		st.Status = "connected"
	default:
		st.Status = "disconnected"
	}
	if jid := s.client.Store.ID; jid != nil {
		st.JID = Identifier(*jid)
		st.Name = s.client.Store.PushName
	}
	return st
}

// StartPairing opens a QR channel (if not already pairing) and returns the
// current QR code, waiting briefly for the first one.
func (s *Session) StartPairing(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.client.Store.ID != nil {
		s.mu.Unlock()
		return "", whatsmeow.ErrQRStoreContainsID
	}
	if s.pairing {
		qr := s.lastQR
		s.mu.Unlock()
		return qr, nil
	}
	s.pairing = true
	s.mu.Unlock()

	// The QR channel must outlive the HTTP request.
	qrChan, err := s.client.GetQRChannel(context.Background())
	if err != nil {
		s.setPairing(false)
		return "", err
	}
	if !s.client.IsConnected() {
		if err := s.client.Connect(); err != nil {
			s.setPairing(false)
			return "", err
		}
	}
	first := make(chan string, 1)
	go func() {
		sent := false
		for item := range qrChan {
			switch item.Event {
			case whatsmeow.QRChannelEventCode:
				s.mu.Lock()
				s.lastQR = item.Code
				s.mu.Unlock()
				if !sent {
					first <- item.Code
					sent = true
				}
			default:
				s.log.Infof("pairing finished: %s", item.Event)
				s.setPairing(false)
				if item.Event != whatsmeow.QRChannelSuccess.Event {
					s.client.Disconnect()
					s.notify(&WebhookPayload{Session: &SessionPayload{Status: "logged_out", Reason: item.Event}})
				}
			}
		}
		if !sent {
			close(first)
		}
	}()
	select {
	case code, ok := <-first:
		if !ok {
			return "", fmt.Errorf("pairing ended before a QR code was issued")
		}
		return code, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(20 * time.Second):
		return "", fmt.Errorf("timed out waiting for QR code")
	}
}

func (s *Session) setPairing(v bool) {
	s.mu.Lock()
	s.pairing = v
	if !v {
		s.lastQR = ""
	}
	s.mu.Unlock()
}

// PairPhone returns an 8-character code to type on the phone.
func (s *Session) PairPhone(ctx context.Context, phone string) (string, error) {
	if _, err := s.StartPairing(ctx); err != nil {
		return "", err
	}
	return s.client.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
}

func (s *Session) Reconnect() error {
	if s.client.Store.ID == nil {
		return fmt.Errorf("session is not paired")
	}
	s.client.Disconnect()
	return s.client.Connect()
}

func (s *Session) notify(p *WebhookPayload) {
	s.mu.Lock()
	rec := s.rec
	s.mu.Unlock()
	if err := s.webhook.Send(context.Background(), rec.WebhookURL, rec.WebhookSecret, p); err != nil {
		s.log.Errorf("webhook delivery failed permanently: %v", err)
	}
}

func (s *Session) resolveJID(jid types.JID) types.JID {
	jid = jid.ToNonAD()
	if jid.Server != types.HiddenUserServer {
		return jid
	}
	pn, err := s.client.Store.LIDs.GetPNForLID(context.Background(), jid)
	if err != nil || pn.IsEmpty() {
		return jid
	}
	return pn.ToNonAD()
}

func (s *Session) lookupGroupName(jid types.JID) string {
	s.mu.Lock()
	name, ok := s.groupName[jid]
	s.mu.Unlock()
	if ok {
		return name
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := s.client.GetGroupInfo(ctx, jid)
	if err != nil {
		s.log.Warnf("group info %s: %v", jid, err)
		return ""
	}
	s.mu.Lock()
	s.groupName[jid] = info.Name
	s.mu.Unlock()
	return info.Name
}

func (s *Session) handleEvent(rawEvt any) {
	switch evt := rawEvt.(type) {
	case *events.Message:
		s.handleMessage(evt, false)
	case *events.Receipt:
		if !evt.IsFromMe || evt.Type == types.ReceiptTypeServerError {
			if p := BuildReceiptPayload(evt); p != nil {
				go s.notify(p)
			}
		}
	case *events.PairSuccess:
		s.log.Infof("paired as %s", evt.ID)
		if err := s.store.SetSessionDevice(context.Background(), s.id, evt.ID.String()); err != nil {
			s.log.Errorf("persist device: %v", err)
		}
	case *events.Connected:
		st := s.Status()
		go s.notify(&WebhookPayload{Session: &SessionPayload{Status: "connected", JID: st.JID, Name: st.Name}})
	case *events.LoggedOut:
		s.log.Warnf("logged out: %v", evt.Reason)
		_ = s.store.SetSessionDevice(context.Background(), s.id, "")
		go s.notify(&WebhookPayload{Session: &SessionPayload{Status: "logged_out", Reason: evt.Reason.String()}})
	case *events.TemporaryBan:
		go s.notify(&WebhookPayload{Session: &SessionPayload{Status: "disconnected", Reason: evt.String()}})
	case *events.HistorySync:
		go s.handleHistory(evt)
	}
}

func (s *Session) handleMessage(evt *events.Message, history bool) {
	ctx := context.Background()
	if err := s.store.SaveMessage(ctx, StoredMessage{
		SessionID: s.id, ID: evt.Info.ID, Chat: evt.Info.Chat.String(), Sender: evt.Info.Sender.ToNonAD().String(),
		FromMe: evt.Info.IsFromMe, Message: evt.Message, CreatedAt: evt.Info.Timestamp,
	}); err != nil {
		s.log.Warnf("cache message %s: %v", evt.Info.ID, err)
	}
	p := BuildMessagePayload(evt, s.resolveJID, s.lookupGroupName, history)
	if p == nil {
		return
	}
	if history {
		s.notify(p)
	} else {
		go s.notify(p)
	}
}

func (s *Session) handleHistory(evt *events.HistorySync) {
	s.mu.Lock()
	importHistory := s.rec.ImportHistory
	s.mu.Unlock()
	if !importHistory {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -s.mgr.cfg.HistoryDays)
	for _, conv := range evt.Data.GetConversations() {
		chat, err := types.ParseJID(conv.GetID())
		if err != nil {
			continue
		}
		msgs := conv.GetMessages()
		// History arrives newest first; deliver oldest first.
		for i := len(msgs) - 1; i >= 0; i-- {
			parsed, err := s.client.ParseWebMessage(chat, msgs[i].GetMessage())
			if err != nil || parsed.Info.Timestamp.Before(cutoff) {
				continue
			}
			s.handleMessage(parsed, true)
		}
	}
}

// Media downloads the media of a cached message, retrying through the phone
// when the CDN copy has expired.
func (s *Session) Media(ctx context.Context, msgID string) ([]byte, string, string, error) {
	stored, err := s.store.GetMessage(ctx, s.id, msgID)
	if err != nil {
		return nil, "", "", err
	}
	msg := stored.Message
	data, err := s.client.DownloadAny(ctx, msg)
	if errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith404) || errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith410) {
		data, err = s.retryMedia(ctx, stored)
	}
	if err != nil {
		return nil, "", "", err
	}
	mime, filename := mediaMeta(msg)
	return data, mime, filename, nil
}

func (s *Session) retryMedia(ctx context.Context, stored *StoredMessage) ([]byte, error) {
	dl, ok := downloadable(stored.Message)
	if !ok {
		return nil, fmt.Errorf("message has no media")
	}
	chat, _ := types.ParseJID(stored.Chat)
	sender, _ := types.ParseJID(stored.Sender)
	info := &types.MessageInfo{ID: stored.ID, MessageSource: types.MessageSource{
		Chat: chat, Sender: sender, IsFromMe: stored.FromMe, IsGroup: chat.Server == types.GroupServer}}

	result := make(chan *events.MediaRetry, 1)
	handlerID := s.client.AddEventHandler(func(e any) {
		if mr, ok := e.(*events.MediaRetry); ok && mr.MessageID == stored.ID {
			select {
			case result <- mr:
			default:
			}
		}
	})
	defer s.client.RemoveEventHandler(handlerID)

	if err := s.client.SendMediaRetryReceipt(ctx, info, dl.GetMediaKey()); err != nil {
		return nil, err
	}
	select {
	case mr := <-result:
		notif, err := whatsmeow.DecryptMediaRetryNotification(mr, dl.GetMediaKey())
		if err != nil {
			return nil, err
		}
		if notif.GetDirectPath() == "" {
			return nil, fmt.Errorf("media no longer available on phone")
		}
		setDirectPath(stored.Message, notif.GetDirectPath())
		_ = s.store.SaveMessage(ctx, *stored)
		return s.client.DownloadAny(ctx, stored.Message)
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timed out waiting for media retry")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type mediaKeyed interface {
	whatsmeow.DownloadableMessage
	GetMimetype() string
}

func downloadable(msg *waE2E.Message) (mediaKeyed, bool) {
	switch {
	case msg.GetImageMessage() != nil:
		return msg.GetImageMessage(), true
	case msg.GetVideoMessage() != nil:
		return msg.GetVideoMessage(), true
	case msg.GetAudioMessage() != nil:
		return msg.GetAudioMessage(), true
	case msg.GetDocumentMessage() != nil:
		return msg.GetDocumentMessage(), true
	case msg.GetStickerMessage() != nil:
		return msg.GetStickerMessage(), true
	}
	return nil, false
}

func setDirectPath(msg *waE2E.Message, path string) {
	switch {
	case msg.GetImageMessage() != nil:
		msg.ImageMessage.DirectPath = &path
	case msg.GetVideoMessage() != nil:
		msg.VideoMessage.DirectPath = &path
	case msg.GetAudioMessage() != nil:
		msg.AudioMessage.DirectPath = &path
	case msg.GetDocumentMessage() != nil:
		msg.DocumentMessage.DirectPath = &path
	case msg.GetStickerMessage() != nil:
		msg.StickerMessage.DirectPath = &path
	}
}

func mediaMeta(msg *waE2E.Message) (mime, filename string) {
	if d, ok := downloadable(msg); ok {
		mime = d.GetMimetype()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		filename = doc.GetFileName()
	}
	return mime, filename
}

func (s *Session) Avatar(ctx context.Context, id string) (string, error) {
	jid, err := ParseIdentifier(id)
	if err != nil {
		return "", err
	}
	info, err := s.client.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{})
	if err != nil || info == nil {
		return "", err
	}
	return info.URL, nil
}

func (s *Session) GroupInfo(ctx context.Context, id string) (*types.GroupInfo, error) {
	jid, err := ParseIdentifier(id)
	if err != nil {
		return nil, err
	}
	return s.client.GetGroupInfo(ctx, jid)
}
