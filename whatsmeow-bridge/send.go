package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

const maxMediaBytes = 64 << 20

type SendRequest struct {
	To          string       `json:"to"`
	Type        string       `json:"type"` // text | image | video | audio | voice | document | sticker | interactive
	Text        string       `json:"text,omitempty"`
	MediaURL    string       `json:"media_url,omitempty"`
	Filename    string       `json:"filename,omitempty"`
	Mime        string       `json:"mime,omitempty"`
	ReplyToID   string       `json:"reply_to_id,omitempty"`
	Interactive *Interactive `json:"interactive,omitempty"`
	// InteractiveMode is "native" (try buttons, fall back to text) or "text".
	InteractiveMode string `json:"interactive_mode,omitempty"`
}

type SendResult struct {
	ID       string `json:"id"`
	Fallback bool   `json:"fallback,omitempty"`
}

var httpClient = &http.Client{Timeout: 60 * time.Second}

func (s *Session) Send(ctx context.Context, req SendRequest) (*SendResult, error) {
	to, err := ParseIdentifier(req.To)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient: %w", err)
	}
	ctxInfo, err := s.quoteContext(ctx, req.ReplyToID)
	if err != nil {
		return nil, err
	}

	if req.Type == "interactive" && req.Interactive != nil {
		return s.sendInteractive(ctx, to, req, ctxInfo)
	}

	var msg *waE2E.Message
	if req.Type == "text" || req.Type == "" {
		msg = textMessage(req.Text, ctxInfo)
	} else {
		msg, err = s.mediaMessage(ctx, req, ctxInfo)
		if err != nil {
			return nil, err
		}
	}
	id, err := s.sendAndStore(ctx, to, msg)
	if err != nil {
		return nil, err
	}
	return &SendResult{ID: id}, nil
}

func (s *Session) sendInteractive(ctx context.Context, to types.JID, req SendRequest, ctxInfo *waE2E.ContextInfo) (*SendResult, error) {
	if req.InteractiveMode != "text" {
		native, err := BuildNativeInteractive(req.Text, req.Interactive)
		if err == nil {
			native.GetViewOnceMessage().GetMessage().GetInteractiveMessage().ContextInfo = ctxInfo
			if id, sendErr := s.sendAndStore(ctx, to, native); sendErr == nil {
				return &SendResult{ID: id}, nil
			} else {
				s.log.Warnf("native interactive send failed, falling back to text: %v", sendErr)
			}
		}
	}
	id, err := s.sendAndStore(ctx, to, textMessage(NumberedFallback(req.Text, req.Interactive), ctxInfo))
	if err != nil {
		return nil, err
	}
	return &SendResult{ID: id, Fallback: true}, nil
}

func (s *Session) sendAndStore(ctx context.Context, to types.JID, msg *waE2E.Message) (string, error) {
	resp, err := s.client.SendMessage(ctx, to, msg)
	if err != nil {
		return "", err
	}
	if err := s.store.SaveMessage(ctx, StoredMessage{
		SessionID: s.id, ID: resp.ID, Chat: to.String(), Sender: s.client.Store.GetJID().ToNonAD().String(),
		FromMe: true, Message: msg, CreatedAt: resp.Timestamp,
	}); err != nil {
		s.log.Warnf("failed to cache sent message %s: %v", resp.ID, err)
	}
	return resp.ID, nil
}

func textMessage(text string, ctxInfo *waE2E.ContextInfo) *waE2E.Message {
	if ctxInfo == nil {
		return &waE2E.Message{Conversation: proto.String(text)}
	}
	return &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(text), ContextInfo: ctxInfo}}
}

// quoteContext builds the ContextInfo for a reply using the cached original.
func (s *Session) quoteContext(ctx context.Context, replyToID string) (*waE2E.ContextInfo, error) {
	if replyToID == "" {
		return nil, nil
	}
	orig, err := s.store.GetMessage(ctx, s.id, replyToID)
	if errors.Is(err, ErrNotFound) {
		// Quoting still works client-side with only the stanza id.
		return &waE2E.ContextInfo{StanzaID: proto.String(replyToID)}, nil
	} else if err != nil {
		return nil, err
	}
	return &waE2E.ContextInfo{
		StanzaID:      proto.String(replyToID),
		Participant:   proto.String(orig.Sender),
		QuotedMessage: orig.Message,
	}, nil
}

func (s *Session) mediaMessage(ctx context.Context, req SendRequest, ctxInfo *waE2E.ContextInfo) (*waE2E.Message, error) {
	data, mime, err := fetchMedia(ctx, req.MediaURL)
	if err != nil {
		return nil, err
	}
	if req.Mime != "" {
		mime = req.Mime
	}
	mediaType := map[string]whatsmeow.MediaType{
		"image": whatsmeow.MediaImage, "sticker": whatsmeow.MediaImage, "video": whatsmeow.MediaVideo,
		"audio": whatsmeow.MediaAudio, "voice": whatsmeow.MediaAudio, "document": whatsmeow.MediaDocument,
	}[req.Type]
	if mediaType == "" {
		return nil, fmt.Errorf("unsupported message type %q", req.Type)
	}
	up, err := s.client.Upload(ctx, data, mediaType)
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	size := uint64(len(data))
	switch req.Type {
	case "image":
		return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256,
			FileSHA256: up.FileSHA256, FileLength: &size, Mimetype: &mime, Caption: optional(req.Text), ContextInfo: ctxInfo,
		}}, nil
	case "sticker":
		return &waE2E.Message{StickerMessage: &waE2E.StickerMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256,
			FileSHA256: up.FileSHA256, FileLength: &size, Mimetype: &mime, ContextInfo: ctxInfo,
		}}, nil
	case "video":
		return &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256,
			FileSHA256: up.FileSHA256, FileLength: &size, Mimetype: &mime, Caption: optional(req.Text), ContextInfo: ctxInfo,
		}}, nil
	case "audio", "voice":
		ptt := req.Type == "voice" || strings.Contains(mime, "ogg")
		if ptt {
			mime = "audio/ogg; codecs=opus"
		}
		return &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256,
			FileSHA256: up.FileSHA256, FileLength: &size, Mimetype: &mime, PTT: proto.Bool(ptt), ContextInfo: ctxInfo,
		}}, nil
	default:
		filename := req.Filename
		if filename == "" {
			filename = "file"
		}
		return &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256,
			FileSHA256: up.FileSHA256, FileLength: &size, Mimetype: &mime, FileName: &filename, Title: &filename,
			Caption: optional(req.Text), ContextInfo: ctxInfo,
		}}, nil
	}
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func fetchMedia(ctx context.Context, url string) ([]byte, string, error) {
	if url == "" {
		return nil, "", fmt.Errorf("media_url is required for media messages")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download media: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("download media: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMediaBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxMediaBytes {
		return nil, "", fmt.Errorf("media larger than %d bytes", maxMediaBytes)
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = http.DetectContentType(data)
	}
	return data, mime, nil
}

func (s *Session) React(ctx context.Context, chatID, senderID, messageID, emoji string) (string, error) {
	chat, err := ParseIdentifier(chatID)
	if err != nil {
		return "", err
	}
	sender := s.client.Store.GetJID().ToNonAD()
	if senderID != "" {
		if sender, err = ParseIdentifier(senderID); err != nil {
			return "", err
		}
	}
	resp, err := s.client.SendMessage(ctx, chat, s.client.BuildReaction(chat, sender, messageID, emoji))
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (s *Session) MarkRead(ctx context.Context, chatID, senderID string, ids []string) error {
	chat, err := ParseIdentifier(chatID)
	if err != nil {
		return err
	}
	sender := chat
	if senderID != "" {
		if sender, err = ParseIdentifier(senderID); err != nil {
			return err
		}
	}
	return s.client.MarkRead(ctx, ids, time.Now(), chat, sender)
}
