package glpi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestClientReinitialisesExpiredSession(t *testing.T) {
	var inits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/initSession":
			n := inits.Add(1)
			_, _ = w.Write([]byte(`{"session_token":"s` + string(rune('0'+n)) + `"}`))
		case "/Computer/7":
			if r.Header.Get("Session-Token") != "s2" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`["ERROR_SESSION_TOKEN_INVALID","expired"]`))
				return
			}
			_, _ = w.Write([]byte(`{"id":7,"name":"x"}`))
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, UserToken: "u"})
	// Prime a stale session, then expect exactly one re-init on the 401.
	c.session = "s1"
	inits.Store(1)

	item, err := c.Get(context.Background(), "Computer", 7)
	if err != nil {
		t.Fatal(err)
	}
	if item["name"] != "x" || inits.Load() != 2 {
		t.Fatalf("item=%v inits=%d", item, inits.Load())
	}
}

func TestClientParsesGLPIErrorTuple(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/initSession" {
			_, _ = w.Write([]byte(`{"session_token":"s"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`["ERROR_ITEM_NOT_FOUND","Item not found"]`))
	}))
	defer srv.Close()

	_, err := New(Config{BaseURL: srv.URL, UserToken: "u"}).Get(context.Background(), "Computer", 1)
	if !IsNotFound(err) {
		t.Fatalf("want not-found, got %v", err)
	}
}
