package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jungley/led/config"
)

func TestNewTelegramNilWhenUnconfigured(t *testing.T) {
	if NewTelegram(&config.Config{}) != nil {
		t.Error("expected nil notifier when unconfigured")
	}
	if NewTelegram(&config.Config{TelegramBotToken: "t"}) != nil {
		t.Error("expected nil notifier when chat id missing")
	}
	tg := NewTelegram(&config.Config{TelegramBotToken: "t", TelegramChatID: "c"})
	if tg == nil {
		t.Fatal("expected non-nil notifier when fully configured")
	}
}

func TestTelegramNotifySendsMessage(t *testing.T) {
	var gotPath, gotChat, gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		var m struct {
			ChatID string `json:"chat_id"`
			Text   string `json:"text"`
		}
		_ = json.Unmarshal(body, &m)
		gotChat = m.ChatID
		gotText = m.Text
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tg := &Telegram{token: "BOTTOKEN", chatID: "12345", base: srv.URL, hc: srv.Client()}
	if err := tg.Notify(context.Background(), "hello"); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if !strings.Contains(gotPath, "/botBOTTOKEN/sendMessage") {
		t.Errorf("path = %q", gotPath)
	}
	if gotChat != "12345" || gotText != "hello" {
		t.Errorf("chat=%q text=%q", gotChat, gotText)
	}
}

func TestTelegramNilNotifyIsNoop(t *testing.T) {
	var tg *Telegram
	if err := tg.Notify(context.Background(), "x"); err != nil {
		t.Errorf("nil Notify should be no-op, got %v", err)
	}
}

func TestTelegramNotifyErrorsOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	tg := &Telegram{token: "t", chatID: "c", base: srv.URL, hc: srv.Client()}
	if err := tg.Notify(context.Background(), "x"); err == nil {
		t.Fatal("expected error on non-200 status")
	}
}
