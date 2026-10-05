package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

var (
	contactJID = types.NewJID("5511999990000", types.DefaultUserServer)
	groupJID   = types.NewJID("120363000000000000", types.GroupServer)
	lidJID     = types.NewJID("98765432101234", types.HiddenUserServer)
)

func identity(j types.JID) types.JID { return j }
func noGroup(types.JID) string       { return "" }

func msgEvent(info types.MessageInfo, m *waE2E.Message) *events.Message {
	if info.ID == "" {
		info.ID = "MSGID"
	}
	if info.Timestamp.IsZero() {
		info.Timestamp = time.Unix(1700000000, 0)
	}
	return &events.Message{Info: info, Message: m}
}

func TestBuildMessagePayloadText(t *testing.T) {
	evt := msgEvent(types.MessageInfo{MessageSource: types.MessageSource{Chat: contactJID, Sender: contactJID}, PushName: "Joe"},
		&waE2E.Message{Conversation: proto.String("hello")})
	p := BuildMessagePayload(evt, identity, noGroup, false)
	if p == nil {
		t.Fatal("expected payload")
	}
	m := p.Messages[0]
	if m.Type != "text" || m.Text.Body != "hello" || m.From != "5511999990000" || m.Timestamp != "1700000000" {
		t.Fatalf("unexpected message: %+v", m)
	}
	if p.Contacts[0].WaID != "5511999990000" || p.Contacts[0].Profile.Name != "Joe" {
		t.Fatalf("unexpected contact: %+v", p.Contacts[0])
	}
}

func TestBuildMessagePayloadReplyAndMedia(t *testing.T) {
	ci := &waE2E.ContextInfo{StanzaID: proto.String("QUOTED")}
	evt := msgEvent(types.MessageInfo{MessageSource: types.MessageSource{Chat: contactJID, Sender: contactJID}},
		&waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("pic"), Mimetype: proto.String("image/jpeg"), ContextInfo: ci}})
	m := BuildMessagePayload(evt, identity, noGroup, false).Messages[0]
	if m.Type != "image" || m.Image.ID != "MSGID" || m.Image.Caption != "pic" || m.Context.ID != "QUOTED" {
		t.Fatalf("unexpected message: %+v / %+v", m, m.Image)
	}
}

func TestBuildMessagePayloadVoiceNote(t *testing.T) {
	evt := msgEvent(types.MessageInfo{MessageSource: types.MessageSource{Chat: contactJID, Sender: contactJID}},
		&waE2E.Message{AudioMessage: &waE2E.AudioMessage{PTT: proto.Bool(true), Mimetype: proto.String("audio/ogg")}})
	m := BuildMessagePayload(evt, identity, noGroup, false).Messages[0]
	if m.Type != "voice" || m.Voice == nil {
		t.Fatalf("expected voice, got %+v", m)
	}
}

func TestBuildMessagePayloadGroup(t *testing.T) {
	evt := msgEvent(types.MessageInfo{MessageSource: types.MessageSource{Chat: groupJID, Sender: contactJID, IsGroup: true}, PushName: "Joe"},
		&waE2E.Message{Conversation: proto.String("hi all")})
	p := BuildMessagePayload(evt, identity, func(types.JID) string { return "Team" }, false)
	m := p.Messages[0]
	if m.From != "120363000000000000@g.us" || m.Group.Participant != "5511999990000" || m.Group.ParticipantName != "Joe" {
		t.Fatalf("unexpected group message: %+v %+v", m, m.Group)
	}
	if p.Contacts[0].WaID != "120363000000000000@g.us" || p.Contacts[0].Profile.Name != "Team" {
		t.Fatalf("group should be the contact: %+v", p.Contacts[0])
	}
}

func TestBuildMessagePayloadResolvesLID(t *testing.T) {
	resolve := func(j types.JID) types.JID {
		if j == lidJID {
			return contactJID
		}
		return j
	}
	evt := msgEvent(types.MessageInfo{MessageSource: types.MessageSource{Chat: lidJID, Sender: lidJID}},
		&waE2E.Message{Conversation: proto.String("x")})
	if from := BuildMessagePayload(evt, resolve, noGroup, false).Messages[0].From; from != "5511999990000" {
		t.Fatalf("expected phone number, got %s", from)
	}
	if from := BuildMessagePayload(evt, identity, noGroup, false).Messages[0].From; from != "98765432101234@lid" {
		t.Fatalf("expected lid identifier, got %s", from)
	}
}

func TestBuildMessagePayloadFromMe(t *testing.T) {
	evt := msgEvent(types.MessageInfo{MessageSource: types.MessageSource{Chat: contactJID, Sender: contactJID, IsFromMe: true}, PushName: "Me"},
		&waE2E.Message{Conversation: proto.String("sent from phone")})
	p := BuildMessagePayload(evt, identity, noGroup, false)
	if !p.Messages[0].FromMe || p.Messages[0].To != "5511999990000" || p.Contacts[0].Profile.Name != "" {
		t.Fatalf("unexpected from_me payload: %+v %+v", p.Messages[0], p.Contacts[0])
	}
}

func TestBuildMessagePayloadReactionAndInteractive(t *testing.T) {
	src := types.MessageSource{Chat: contactJID, Sender: contactJID}
	r := BuildMessagePayload(msgEvent(types.MessageInfo{MessageSource: src},
		&waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Key: &waCommonKey, Text: proto.String("👍")}}), identity, noGroup, false)
	if r.Messages[0].Type != "reaction" || r.Messages[0].Reaction.MessageID != "TARGET" || r.Messages[0].Reaction.Emoji != "👍" {
		t.Fatalf("unexpected reaction: %+v", r.Messages[0])
	}

	nf := &waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{
		Body: &waE2E.InteractiveResponseMessage_Body{Text: proto.String("Yes")},
		InteractiveResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage_{
			NativeFlowResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage{
				Name: proto.String("quick_reply"), ParamsJSON: proto.String(`{"id":"opt_yes"}`)}},
	}}
	i := BuildMessagePayload(msgEvent(types.MessageInfo{MessageSource: src}, nf), identity, noGroup, false).Messages[0]
	if i.Type != "interactive" || i.Interactive.ButtonReply.ID != "opt_yes" || i.Interactive.ButtonReply.Title != "Yes" {
		t.Fatalf("unexpected interactive reply: %+v", i.Interactive)
	}
}

func TestBuildMessagePayloadSkipsUnsupported(t *testing.T) {
	src := types.MessageSource{Chat: contactJID, Sender: contactJID}
	if p := BuildMessagePayload(msgEvent(types.MessageInfo{MessageSource: src},
		&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{}}), identity, noGroup, false); p != nil {
		t.Fatal("protocol messages must be skipped")
	}
	status := types.MessageSource{Chat: types.StatusBroadcastJID, Sender: contactJID}
	if p := BuildMessagePayload(msgEvent(types.MessageInfo{MessageSource: status},
		&waE2E.Message{Conversation: proto.String("story")}), identity, noGroup, false); p != nil {
		t.Fatal("status broadcasts must be skipped")
	}
}

func TestBuildMessagePayloadContacts(t *testing.T) {
	vcard := "BEGIN:VCARD\nVERSION:3.0\nFN:Ana\nTEL;type=CELL;waid=5511988887777:+55 11 98888-7777\nEND:VCARD"
	evt := msgEvent(types.MessageInfo{MessageSource: types.MessageSource{Chat: contactJID, Sender: contactJID}},
		&waE2E.Message{ContactMessage: &waE2E.ContactMessage{DisplayName: proto.String("Ana"), Vcard: proto.String(vcard)}})
	m := BuildMessagePayload(evt, identity, noGroup, false).Messages[0]
	if m.Type != "contacts" || m.Contacts[0].Name.FormattedName != "Ana" || m.Contacts[0].Phones[0].Phone != "+55 11 98888-7777" {
		t.Fatalf("unexpected contacts: %+v", m.Contacts)
	}
}

func TestBuildReceiptPayload(t *testing.T) {
	cases := map[types.ReceiptType]string{
		types.ReceiptTypeDelivered: "delivered", types.ReceiptTypeRead: "read", types.ReceiptTypePlayed: "read",
	}
	for rt, want := range cases {
		p := BuildReceiptPayload(&events.Receipt{Type: rt, MessageIDs: []types.MessageID{"A", "B"}, Timestamp: time.Unix(1, 0)})
		if len(p.Statuses) != 2 || p.Statuses[0].Status != want {
			t.Fatalf("receipt %q: got %+v", rt, p)
		}
	}
	if BuildReceiptPayload(&events.Receipt{Type: types.ReceiptTypeRetry}) != nil {
		t.Fatal("retry receipts must be ignored")
	}
}

func TestIdentifierRoundTrip(t *testing.T) {
	for _, id := range []string{"5511999990000", "120363000000000000@g.us", "98765432101234@lid"} {
		jid, err := ParseIdentifier(id)
		if err != nil || Identifier(jid) != id {
			t.Fatalf("round trip failed for %s: %v %v", id, jid, err)
		}
	}
	if jid, _ := ParseIdentifier("+5511999990000"); jid != contactJID {
		t.Fatalf("leading plus should be stripped: %v", jid)
	}
}

func TestPayloadJSONShape(t *testing.T) {
	evt := msgEvent(types.MessageInfo{MessageSource: types.MessageSource{Chat: contactJID, Sender: contactJID}},
		&waE2E.Message{Conversation: proto.String("hi")})
	raw, _ := json.Marshal(BuildMessagePayload(evt, identity, noGroup, true))
	s := string(raw)
	for _, want := range []string{`"wa_id":"5511999990000"`, `"type":"text"`, `"text":{"body":"hi"}`, `"history":true`} {
		if !strings.Contains(s, want) {
			t.Fatalf("payload %s missing %s", s, want)
		}
	}
	if strings.Contains(s, `"statuses"`) || strings.Contains(s, `"session"`) {
		t.Fatalf("empty sections must be omitted: %s", s)
	}
}
