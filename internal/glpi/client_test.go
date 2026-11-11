package glpi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func newServer(t *testing.T, inits *atomic.Int32, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/initSession" {
			_, _ = w.Write([]byte(`{"session_token":"s` + strconv.Itoa(int(inits.Add(1))) + `"}`))
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return New(Config{BaseURL: srv.URL, UserToken: "u", AppToken: "app"})
}

func TestClientReinitialisesExpiredSessionOnce(t *testing.T) {
	var inits atomic.Int32
	var calls atomic.Int32
	c := newServer(t, &inits, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Session-Token") != "s2" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`["ERROR_SESSION_TOKEN_INVALID","expired"]`))
			return
		}
		_, _ = w.Write([]byte(`{"id":7,"name":"x"}`))
	})
	c.session = "s1"
	inits.Store(1)

	item, err := c.Get(context.Background(), "Computer", 7)
	if err != nil {
		t.Fatal(err)
	}
	if item["name"] != "x" || inits.Load() != 2 || calls.Load() != 2 {
		t.Fatalf("item=%v inits=%d calls=%d", item, inits.Load(), calls.Load())
	}
}

func TestClientDoesNotLoopOnPersistent401(t *testing.T) {
	var inits, calls atomic.Int32
	c := newServer(t, &inits, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`["ERROR_GLPI_LOGIN","nope"]`))
	})
	if _, err := c.Get(context.Background(), "Computer", 1); err == nil {
		t.Fatal("want error")
	}
	if calls.Load() != 2 || inits.Load() != 2 {
		t.Fatalf("calls=%d inits=%d, want exactly one retry", calls.Load(), inits.Load())
	}
}

func TestIsNotFoundNeedsItemNotFoundCode(t *testing.T) {
	var inits atomic.Int32
	code := "ERROR_ITEM_NOT_FOUND"
	c := newServer(t, &inits, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`["` + code + `","Item not found"]`))
	})
	if _, err := c.Get(context.Background(), "Computer", 1); !IsNotFound(err) {
		t.Fatalf("missing item must be not-found, got %v", err)
	}
	code = "ERROR_RESOURCE_NOT_FOUND_NOR_COMMONDBTM"
	if _, err := c.Get(context.Background(), "Bogus", 1); err == nil || IsNotFound(err) {
		t.Fatalf("unknown itemtype must stay an error, got %v", err)
	}
}

func TestInitSessionFailureIsNeverNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`["ERROR_ITEM_NOT_FOUND","wrong vhost"]`))
	}))
	defer srv.Close()
	_, err := New(Config{BaseURL: srv.URL, UserToken: "u"}).Get(context.Background(), "Computer", 1)
	if err == nil || !strings.Contains(err.Error(), "apirest.php") {
		t.Fatalf("want an initSession error with a URL hint, got %v", err)
	}
}

func TestInitSessionRejectsEmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	_, err := New(Config{BaseURL: srv.URL, UserToken: "u"}).Get(context.Background(), "Computer", 1)
	if err == nil || !strings.Contains(err.Error(), "no session_token") {
		t.Fatalf("got %v", err)
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	var followed atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followed.Store(true)
	}))
	defer other.Close()
	var inits atomic.Int32
	c := newServer(t, &inits, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/x", http.StatusFound)
	})
	_, err := c.Create(context.Background(), "Computer", Item{"name": "x"})
	if err == nil || !strings.Contains(err.Error(), "redirected to") {
		t.Fatalf("got %v", err)
	}
	if followed.Load() {
		t.Fatal("redirect target was contacted with session headers")
	}
}

func TestClientSendsAuthHeadersAndDeleteMode(t *testing.T) {
	for _, purge := range []bool{false, true} {
		var gotPurge, gotApp, gotSession string
		var inits atomic.Int32
		c := newServer(t, &inits, func(w http.ResponseWriter, r *http.Request) {
			gotPurge = r.URL.Query().Get("force_purge")
			gotApp = r.Header.Get("App-Token")
			gotSession = r.Header.Get("Session-Token")
			_, _ = w.Write([]byte(`[{"5":true,"message":""}]`))
		})
		c.cfg.PurgeOnDestroy = purge
		if err := c.Delete(context.Background(), "Computer", 5); err != nil {
			t.Fatal(err)
		}
		if gotPurge != strconv.FormatBool(purge) || gotApp != "app" || gotSession != "s1" {
			t.Fatalf("purge=%v: force_purge=%q app=%q session=%q", purge, gotPurge, gotApp, gotSession)
		}
	}
}

func TestUpdateSurfacesRejectedItem(t *testing.T) {
	var inits atomic.Int32
	c := newServer(t, &inits, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"7":false,"message":"You don't have permission"}]`))
	})
	err := c.Update(context.Background(), "Computer", 7, Item{"name": "x"})
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("got %v", err)
	}
}

func TestFindByNameKeepsOnlyExactMatches(t *testing.T) {
	var inits atomic.Int32
	c := newServer(t, &inits, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"name":"web"},{"id":2,"name":"web-02"},{"id":3,"name":"web"}]`))
	})
	got, err := c.FindByName(context.Background(), "Computer", "web")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestCreateRejectsMissingID(t *testing.T) {
	var inits atomic.Int32
	c := newServer(t, &inits, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	})
	if _, err := c.Create(context.Background(), "Computer", Item{"name": "x"}); err == nil {
		t.Fatal("want error for response without id")
	}
}

func TestErrorBodyIsTruncated(t *testing.T) {
	var inits atomic.Int32
	c := newServer(t, &inits, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(strings.Repeat("<html>", 1000)))
	})
	_, err := c.Get(context.Background(), "Computer", 1)
	if err == nil || len(err.Error()) > 700 {
		t.Fatalf("error not truncated: %d bytes", len(err.Error()))
	}
}

func TestSearchSegmentAvoidsCharactersGLPIEscapes(t *testing.T) {
	for in, want := range map[string]string{
		"web-01":             "web-01",
		"R&D lab":            "D lab",
		"a & <company> corp": "company",
		"&&&":                "",
	} {
		if got := searchSegment(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
