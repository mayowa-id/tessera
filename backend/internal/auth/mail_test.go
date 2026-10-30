package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestResendDelivery(t *testing.T) {
	for _, status := range []int{200, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			m := ResendMailer{Key: "private-test-key", From: "Tessera <onboarding@example.com>", Client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://api.resend.com/emails" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer private-test-key" {
					t.Fatal("invalid provider request")
				}
				var body struct {
					From          string
					To            []string
					Subject, Text string
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body.From != "Tessera <onboarding@example.com>" || len(body.To) != 1 || body.To[0] != "user@example.com" || body.Text != "Test link" {
					t.Fatal("invalid email")
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"id":"test-message-id"}`)), Header: make(http.Header)}, nil
			})}}
			err := m.Send(context.Background(), "user@example.com", "Account link", "Test link")
			if (err != nil) != (status != 200) {
				t.Fatalf("status %d: %v", status, err)
			}
		})
	}
}

func TestResendRequiresAcceptanceID(t *testing.T) {
	for _, body := range []string{`{}`, `{"id":""}`, `<html>Proxy error</html>`} {
		m := ResendMailer{Key: "test-key", From: "onboarding@example.com", Client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})}}
		if err := m.Send(context.Background(), "user@example.com", "Verify", "Test"); err == nil {
			t.Fatal("claimed email success without a provider acceptance id")
		}
	}
}
