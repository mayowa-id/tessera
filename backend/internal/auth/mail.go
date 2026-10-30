package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Mailer keeps delivery synchronous so serverless instances may safely stop after a response.
type Mailer interface {
	Send(context.Context, string, string, string) error
}
type ResendMailer struct {
	Key, From string
	Client    *http.Client
}

func (m ResendMailer) Send(ctx context.Context, to, subject, body string) error {
	payload, err := json.Marshal(map[string]any{"from": m.From, "to": []string{to}, "subject": subject, "text": body})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.Key)
	req.Header.Set("Content-Type", "application/json")
	client := m.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("email delivery unavailable")
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("email provider status %d", res.StatusCode)
	}
	return nil
}
