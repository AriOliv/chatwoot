package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	waLog "go.mau.fi/whatsmeow/util/log"
)

const SignatureHeader = "X-Whatsmeow-Signature"

// Sign returns the value for SignatureHeader: "sha256=<hex hmac of body>".
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// WebhookSender delivers payloads to Chatwoot with retries.
type WebhookSender struct {
	client *http.Client
	log    waLog.Logger
}

func NewWebhookSender(log waLog.Logger) *WebhookSender {
	return &WebhookSender{client: &http.Client{Timeout: 30 * time.Second}, log: log}
}

func (w *WebhookSender) Send(ctx context.Context, url, secret string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			}
		}
		lastErr = w.post(ctx, url, secret, body)
		if lastErr == nil {
			return nil
		}
		w.log.Warnf("webhook delivery to %s failed (attempt %d): %v", url, attempt+1, lastErr)
	}
	return lastErr
}

func (w *WebhookSender) post(ctx context.Context, url, secret string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SignatureHeader, Sign(secret, body))
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
