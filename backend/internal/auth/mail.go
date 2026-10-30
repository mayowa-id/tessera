package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	Logger    *slog.Logger
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
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return fmt.Errorf("email provider status %d", res.StatusCode)
	}
	var accepted struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&accepted); err != nil || accepted.ID == "" {
		return fmt.Errorf("email provider did not confirm message acceptance")
	}
	if m.Logger != nil {
		m.Logger.Info("email accepted by Resend", "email_id", accepted.ID)
	}
	return nil
}
