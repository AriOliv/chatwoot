package main

import (
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Payload shapes mirror the flat 360dialog webhook format that Chatwoot's
// Whatsapp::IncomingMessageService already understands, plus a few extensions
// (from_me, history, group, reaction, session).

type WebhookPayload struct {
	Contacts []ContactPayload `json:"contacts,omitempty"`
	Messages []MessagePayload `json:"messages,omitempty"`
	Statuses []StatusPayload  `json:"statuses,omitempty"`
	Session  *SessionPayload  `json:"session,omitempty"`
}

type ContactPayload struct {
	WaID    string         `json:"wa_id"`
	Profile ProfilePayload `json:"profile"`
}

type ProfilePayload struct {
	Name string `json:"name,omitempty"`
}

type MessagePayload struct {
	ID          string              `json:"id"`
	From        string              `json:"from"`
	To          string              `json:"to,omitempty"`
	Timestamp   string              `json:"timestamp"`
	Type        string              `json:"type"`
	FromMe      bool                `json:"from_me,omitempty"`
	History     bool                `json:"history,omitempty"`
	Text        *TextPayload        `json:"text,omitempty"`
	Image       *MediaPayload       `json:"image,omitempty"`
	Video       *MediaPayload       `json:"video,omitempty"`
	Audio       *MediaPayload       `json:"audio,omitempty"`
	Voice       *MediaPayload       `json:"voice,omitempty"`
	Document    *MediaPayload       `json:"document,omitempty"`
	Sticker     *MediaPayload       `json:"sticker,omitempty"`
	Location    *LocationPayload    `json:"location,omitempty"`
	Contacts    []VCardPayload      `json:"contacts,omitempty"`
	Interactive *InteractivePayload `json:"interactive,omitempty"`
	Reaction    *ReactionPayload    `json:"reaction,omitempty"`
	Context     *ContextPayload     `json:"context,omitempty"`
	Group       *GroupPayload       `json:"group,omitempty"`
}

type TextPayload struct {
	Body string `json:"body"`
}

type MediaPayload struct {
	ID       string `json:"id"`
	Caption  string `json:"caption,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type LocationPayload struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name,omitempty"`
	Address   string  `json:"address,omitempty"`
	URL       string  `json:"url,omitempty"`
}

type VCardPayload struct {
	Name   VCardName    `json:"name"`
	Phones []VCardPhone `json:"phones,omitempty"`
}

type VCardName struct {
	FormattedName string `json:"formatted_name"`
	FirstName     string `json:"first_name,omitempty"`
}

type VCardPhone struct {
	Phone string `json:"phone"`
}

type InteractivePayload struct {
	Type        string       `json:"type"`
	ButtonReply *ReplyOption `json:"button_reply,omitempty"`
	ListReply   *ReplyOption `json:"list_reply,omitempty"`
}

type ReplyOption struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type ReactionPayload struct {
	MessageID string `json:"message_id"`
	Emoji     string `json:"emoji"`
}

type ContextPayload struct {
	ID string `json:"id"`
}

type GroupPayload struct {
	ID              string `json:"id"`
	Subject         string `json:"subject,omitempty"`
	Participant     string `json:"participant"`
	ParticipantName string `json:"participant_name,omitempty"`
}

type StatusPayload struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp,omitempty"`
}

type SessionPayload struct {
	Status string `json:"status"`
	JID    string `json:"jid,omitempty"`
	Name   string `json:"name,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// JIDResolver maps LID JIDs to phone-number JIDs when the mapping is known.
type JIDResolver func(types.JID) types.JID

// Identifier returns the Chatwoot-facing identifier for a JID:
// digits only for phone-number users, "<user>@g.us" for groups and
// "<user>@lid" when no phone number is known.
func Identifier(jid types.JID) string {
	jid = jid.ToNonAD()
	switch jid.Server {
	case types.DefaultUserServer:
		return jid.User
	case types.GroupServer, types.HiddenUserServer:
		return jid.User + "@" + jid.Server
	default:
		return jid.String()
	}
}

// ParseIdentifier is the inverse of Identifier.
func ParseIdentifier(id string) (types.JID, error) {
	if strings.Contains(id, "@") {
		return types.ParseJID(id)
	}
	return types.NewJID(strings.TrimPrefix(id, "+"), types.DefaultUserServer), nil
}

// GroupNames looks up a group subject; may return "".
type GroupNames func(types.JID) string

// BuildMessagePayload converts a whatsmeow message event into a webhook payload.
// It returns nil for events Chatwoot has nothing to do with (protocol messages,
// status broadcasts, newsletters, empty messages).
func BuildMessagePayload(evt *events.Message, resolve JIDResolver, groupName GroupNames, history bool) *WebhookPayload {
	info := evt.Info
	if info.Chat.Server == types.BroadcastServer || info.Chat.Server == types.NewsletterServer {
		return nil
	}
	msg := evt.Message
	if msg == nil || msg.GetProtocolMessage() != nil {
		return nil
	}

	chat := resolve(info.Chat)
	sender := resolve(info.Sender)
	if info.SenderAlt.Server == types.DefaultUserServer && sender.Server == types.HiddenUserServer {
		sender = info.SenderAlt
	}

	mp := MessagePayload{
		ID:        info.ID,
		Timestamp: strconv.FormatInt(info.Timestamp.Unix(), 10),
		FromMe:    info.IsFromMe,
		History:   history,
	}
	if !fillContent(&mp, msg) {
		return nil
	}

	contactID := Identifier(chat)
	contactName := info.PushName
	if info.IsGroup {
		subject := groupName(info.Chat)
		mp.Group = &GroupPayload{
			ID:              contactID,
			Subject:         subject,
			Participant:     Identifier(sender),
			ParticipantName: info.PushName,
		}
		contactName = subject
		if contactName == "" {
			contactName = contactID
		}
	}
	mp.From = contactID
	if info.IsFromMe {
		mp.To = contactID
		if !info.IsGroup {
			// PushName on our own messages is our name, not the contact's.
			contactName = ""
		}
	}

	return &WebhookPayload{
		Contacts: []ContactPayload{{WaID: contactID, Profile: ProfilePayload{Name: contactName}}},
		Messages: []MessagePayload{mp},
	}
}

func contextOf(ci *waE2E.ContextInfo) *ContextPayload {
	if ci.GetStanzaID() == "" {
		return nil
	}
	return &ContextPayload{ID: ci.GetStanzaID()}
}

// fillContent sets Type and the type-specific field. Returns false if the
// message carries nothing we can represent.
func fillContent(mp *MessagePayload, msg *waE2E.Message) bool {
	media := func(id, caption, mime, filename string) *MediaPayload {
		return &MediaPayload{ID: id, Caption: caption, MimeType: mime, Filename: filename}
	}
	switch {
	case msg.GetConversation() != "":
		mp.Type, mp.Text = "text", &TextPayload{Body: msg.GetConversation()}
	case msg.GetExtendedTextMessage() != nil:
		m := msg.GetExtendedTextMessage()
		mp.Type, mp.Text = "text", &TextPayload{Body: m.GetText()}
		mp.Context = contextOf(m.GetContextInfo())
	case msg.GetImageMessage() != nil:
		m := msg.GetImageMessage()
		mp.Type, mp.Image = "image", media(mp.ID, m.GetCaption(), m.GetMimetype(), "")
		mp.Context = contextOf(m.GetContextInfo())
	case msg.GetVideoMessage() != nil:
		m := msg.GetVideoMessage()
		mp.Type, mp.Video = "video", media(mp.ID, m.GetCaption(), m.GetMimetype(), "")
		mp.Context = contextOf(m.GetContextInfo())
	case msg.GetAudioMessage() != nil:
		m := msg.GetAudioMessage()
		if m.GetPTT() {
			mp.Type, mp.Voice = "voice", media(mp.ID, "", m.GetMimetype(), "")
		} else {
			mp.Type, mp.Audio = "audio", media(mp.ID, "", m.GetMimetype(), "")
		}
		mp.Context = contextOf(m.GetContextInfo())
	case msg.GetDocumentMessage() != nil:
		m := msg.GetDocumentMessage()
		mp.Type, mp.Document = "document", media(mp.ID, m.GetCaption(), m.GetMimetype(), m.GetFileName())
		mp.Context = contextOf(m.GetContextInfo())
	case msg.GetStickerMessage() != nil:
		m := msg.GetStickerMessage()
		mp.Type, mp.Sticker = "sticker", media(mp.ID, "", m.GetMimetype(), "")
		mp.Context = contextOf(m.GetContextInfo())
	case msg.GetLocationMessage() != nil:
		m := msg.GetLocationMessage()
		mp.Type = "location"
		mp.Location = &LocationPayload{Latitude: m.GetDegreesLatitude(), Longitude: m.GetDegreesLongitude(),
			Name: m.GetName(), Address: m.GetAddress(), URL: m.GetURL()}
	case msg.GetLiveLocationMessage() != nil:
		m := msg.GetLiveLocationMessage()
		mp.Type = "location"
		mp.Location = &LocationPayload{Latitude: m.GetDegreesLatitude(), Longitude: m.GetDegreesLongitude(), Name: m.GetCaption()}
	case msg.GetContactMessage() != nil:
		mp.Type = "contacts"
		mp.Contacts = []VCardPayload{vcardPayload(msg.GetContactMessage())}
	case msg.GetContactsArrayMessage() != nil:
		mp.Type = "contacts"
		for _, c := range msg.GetContactsArrayMessage().GetContacts() {
			mp.Contacts = append(mp.Contacts, vcardPayload(c))
		}
	case msg.GetReactionMessage() != nil:
		m := msg.GetReactionMessage()
		mp.Type = "reaction"
		mp.Reaction = &ReactionPayload{MessageID: m.GetKey().GetID(), Emoji: m.GetText()}
	case msg.GetButtonsResponseMessage() != nil:
		m := msg.GetButtonsResponseMessage()
		mp.Type = "interactive"
		mp.Interactive = &InteractivePayload{Type: "button_reply",
			ButtonReply: &ReplyOption{ID: m.GetSelectedButtonID(), Title: m.GetSelectedDisplayText()}}
		mp.Context = contextOf(m.GetContextInfo())
	case msg.GetTemplateButtonReplyMessage() != nil:
		m := msg.GetTemplateButtonReplyMessage()
		mp.Type = "interactive"
		mp.Interactive = &InteractivePayload{Type: "button_reply",
			ButtonReply: &ReplyOption{ID: m.GetSelectedID(), Title: m.GetSelectedDisplayText()}}
		mp.Context = contextOf(m.GetContextInfo())
	case msg.GetListResponseMessage() != nil:
		m := msg.GetListResponseMessage()
		mp.Type = "interactive"
		mp.Interactive = &InteractivePayload{Type: "list_reply",
			ListReply: &ReplyOption{ID: m.GetSingleSelectReply().GetSelectedRowID(), Title: m.GetTitle()}}
		mp.Context = contextOf(m.GetContextInfo())
	case msg.GetInteractiveResponseMessage() != nil:
		m := msg.GetInteractiveResponseMessage()
		id, title := parseNativeFlowResponse(m)
		mp.Type = "interactive"
		mp.Interactive = &InteractivePayload{Type: "button_reply", ButtonReply: &ReplyOption{ID: id, Title: title}}
		mp.Context = contextOf(m.GetContextInfo())
	case msg.GetPollCreationMessage() != nil || msg.GetPollCreationMessageV3() != nil:
		poll := msg.GetPollCreationMessage()
		if poll == nil {
			poll = msg.GetPollCreationMessageV3()
		}
		lines := []string{"📊 " + poll.GetName()}
		for i, o := range poll.GetOptions() {
			lines = append(lines, strconv.Itoa(i+1)+") "+o.GetOptionName())
		}
		mp.Type, mp.Text = "text", &TextPayload{Body: strings.Join(lines, "\n")}
	default:
		return false
	}
	return true
}

func vcardPayload(c *waE2E.ContactMessage) VCardPayload {
	v := VCardPayload{Name: VCardName{FormattedName: c.GetDisplayName(), FirstName: c.GetDisplayName()}}
	for _, line := range strings.Split(c.GetVcard(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), "TEL") {
			continue
		}
		if idx := strings.LastIndex(line, ":"); idx >= 0 {
			v.Phones = append(v.Phones, VCardPhone{Phone: strings.TrimSpace(line[idx+1:])})
		}
	}
	return v
}

// BuildReceiptPayload maps a receipt to Chatwoot statuses. Returns nil for
// receipt types Chatwoot doesn't track.
func BuildReceiptPayload(evt *events.Receipt) *WebhookPayload {
	var status string
	switch evt.Type {
	case types.ReceiptTypeDelivered:
		status = "delivered"
	case types.ReceiptTypeRead, types.ReceiptTypePlayed:
		status = "read"
	case types.ReceiptTypeServerError:
		status = "failed"
	default:
		return nil
	}
	p := &WebhookPayload{}
	ts := strconv.FormatInt(evt.Timestamp.Unix(), 10)
	for _, id := range evt.MessageIDs {
		p.Statuses = append(p.Statuses, StatusPayload{ID: id, Status: status, Timestamp: ts})
	}
	return p
}
