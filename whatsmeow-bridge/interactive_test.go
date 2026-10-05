package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"google.golang.org/protobuf/proto"
)

var waCommonKey = waCommon.MessageKey{ID: proto.String("TARGET")}

func TestBuildNativeInteractiveButtons(t *testing.T) {
	msg, err := BuildNativeInteractive("Pick one", &Interactive{Footer: "f", Buttons: []ReplyOption{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}}})
	if err != nil {
		t.Fatal(err)
	}
	im := msg.GetViewOnceMessage().GetMessage().GetInteractiveMessage()
	if im.GetBody().GetText() != "Pick one" || im.GetFooter().GetText() != "f" {
		t.Fatalf("unexpected body/footer: %+v", im)
	}
	buttons := im.GetNativeFlowMessage().GetButtons()
	if len(buttons) != 2 || buttons[0].GetName() != "quick_reply" {
		t.Fatalf("unexpected buttons: %+v", buttons)
	}
	var params map[string]string
	_ = json.Unmarshal([]byte(buttons[1].GetButtonParamsJSON()), &params)
	if params["id"] != "b" || params["display_text"] != "B" {
		t.Fatalf("unexpected params: %v", params)
	}
}

func TestBuildNativeInteractiveList(t *testing.T) {
	in := &Interactive{List: &ListSpec{ButtonText: "Menu", Sections: []ListSection{{Title: "S", Rows: []ReplyOption{{ID: "1", Title: "One"}}}}}}
	msg, err := BuildNativeInteractive("Choose", in)
	if err != nil {
		t.Fatal(err)
	}
	btn := msg.GetViewOnceMessage().GetMessage().GetInteractiveMessage().GetNativeFlowMessage().GetButtons()[0]
	if btn.GetName() != "single_select" || !strings.Contains(btn.GetButtonParamsJSON(), `"title":"Menu"`) {
		t.Fatalf("unexpected list button: %+v", btn)
	}
}

func TestBuildNativeInteractiveRequiresOptions(t *testing.T) {
	if _, err := BuildNativeInteractive("x", &Interactive{}); err == nil {
		t.Fatal("expected error without options")
	}
}

func TestNumberedFallback(t *testing.T) {
	got := NumberedFallback("Pick one", &Interactive{Header: "H", Buttons: []ReplyOption{{Title: "A"}, {Title: "B"}}})
	want := "*H*\nPick one\n\n1) A\n2) B"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSign(t *testing.T) {
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(`{"a":1}`))
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got := Sign("secret", []byte(`{"a":1}`)); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
