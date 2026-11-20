package auth

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/sessions"
)

func TestOAuthStoreSecure(t *testing.T) {
	base := sessions.NewCookieStore([]byte("test-key"))
	store := requestSecureStore{base}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.TLS = &tls.ConnectionState{}

	sess, err := store.New(req, "n")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !sess.Options.Secure {
		t.Error("New did not mark the session Secure for a TLS request")
	}

	got, err := store.Get(req, "n")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Options.Secure {
		t.Error("Get did not mark the session Secure for a TLS request")
	}

	rec := httptest.NewRecorder()
	if err := store.Save(req, rec, sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	store.markSecure(req, nil)
}

func TestRequestSecureStoreOverHTTP(t *testing.T) {
	base := sessions.NewCookieStore([]byte("test-key"))
	base.Options.Secure = true
	store := requestSecureStore{base}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sess, _ := store.New(req, "n")
	if sess.Options.Secure {
		t.Error("plain-HTTP session must not be Secure")
	}
}
