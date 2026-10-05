package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// Interactive describes Chatwoot's input_select / cards content in a
// provider-neutral way.
type Interactive struct {
	Header  string        `json:"header,omitempty"`
	Footer  string        `json:"footer,omitempty"`
	Buttons []ReplyOption `json:"buttons,omitempty"`
	List    *ListSpec     `json:"list,omitempty"`
}

type ListSpec struct {
	ButtonText string        `json:"button_text"`
	Sections   []ListSection `json:"sections"`
}

type ListSection struct {
	Title string        `json:"title,omitempty"`
	Rows  []ReplyOption `json:"rows"`
}

func (in *Interactive) options() []ReplyOption {
	if in.List == nil {
		return in.Buttons
	}
	var out []ReplyOption
	for _, s := range in.List.Sections {
		out = append(out, s.Rows...)
	}
	return out
}

// BuildNativeInteractive builds a native-flow interactive message wrapped in
// viewOnceMessage, which is the format WhatsApp clients still render for
// non-business senders (quick_reply buttons / single_select list).
func BuildNativeInteractive(body string, in *Interactive) (*waE2E.Message, error) {
	var buttons []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton
	if in.List != nil {
		params, err := json.Marshal(map[string]any{
			"title":    in.List.ButtonText,
			"sections": nativeSections(in.List.Sections),
		})
		if err != nil {
			return nil, err
		}
		buttons = append(buttons, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
			Name: proto.String("single_select"), ButtonParamsJSON: proto.String(string(params)),
		})
	} else {
		for _, b := range in.Buttons {
			params, err := json.Marshal(map[string]string{"display_text": b.Title, "id": b.ID})
			if err != nil {
				return nil, err
			}
			buttons = append(buttons, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
				Name: proto.String("quick_reply"), ButtonParamsJSON: proto.String(string(params)),
			})
		}
	}
	if len(buttons) == 0 {
		return nil, fmt.Errorf("interactive message without options")
	}

	im := &waE2E.InteractiveMessage{
		Body: &waE2E.InteractiveMessage_Body{Text: proto.String(body)},
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
				Buttons:        buttons,
				MessageVersion: proto.Int32(1),
			},
		},
	}
	if in.Header != "" {
		im.Header = &waE2E.InteractiveMessage_Header{Title: proto.String(in.Header), HasMediaAttachment: proto.Bool(false)}
	}
	if in.Footer != "" {
		im.Footer = &waE2E.InteractiveMessage_Footer{Text: proto.String(in.Footer)}
	}
	return &waE2E.Message{
		ViewOnceMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
			MessageContextInfo: &waE2E.MessageContextInfo{
				DeviceListMetadataVersion: proto.Int32(2),
			},
			InteractiveMessage: im,
		}},
	}, nil
}

func nativeSections(sections []ListSection) []map[string]any {
	out := make([]map[string]any, 0, len(sections))
	for _, s := range sections {
		rows := make([]map[string]string, 0, len(s.Rows))
		for _, r := range s.Rows {
			rows = append(rows, map[string]string{"id": r.ID, "title": r.Title})
		}
		out = append(out, map[string]any{"title": s.Title, "rows": rows})
	}
	return out
}

// NumberedFallback renders the options as plain text the recipient can answer
// with a number. Chatwoot maps the numeric answer back to the option.
func NumberedFallback(body string, in *Interactive) string {
	var b strings.Builder
	if in.Header != "" {
		b.WriteString("*" + in.Header + "*\n")
	}
	b.WriteString(body)
	b.WriteString("\n")
	for i, o := range in.options() {
		b.WriteString("\n" + strconv.Itoa(i+1) + ") " + o.Title)
	}
	if in.Footer != "" {
		b.WriteString("\n\n_" + in.Footer + "_")
	}
	return b.String()
}

// parseNativeFlowResponse extracts the selected option id/title from a
// native-flow response (quick_reply or single_select).
func parseNativeFlowResponse(m *waE2E.InteractiveResponseMessage) (id, title string) {
	title = m.GetBody().GetText()
	nf := m.GetNativeFlowResponseMessage()
	if nf == nil {
		return "", title
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(nf.GetParamsJSON()), &params); err == nil {
		if v, ok := params["id"].(string); ok {
			id = v
		}
		if v, ok := params["title"].(string); ok && title == "" {
			title = v
		}
		if v, ok := params["display_text"].(string); ok && title == "" {
			title = v
		}
	}
	return id, title
}
