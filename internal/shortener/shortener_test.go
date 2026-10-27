package shortener

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	st, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, BaseURL: "http://sho.rt"}
	return s, s.Routes()
}

func do(h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, &buf))
	return rec
}

func TestCreateAndRedirect(t *testing.T) {
	_, h := newServer(t)
	rec := do(h, "POST", "/api/links", map[string]any{"url": "https://lekopien.com/projects"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d %s", rec.Code, rec.Body)
	}
	var got linkResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Code) != 6 || got.ShortURL != "http://sho.rt/"+got.Code {
		t.Fatalf("unexpected link %+v", got)
	}

	rec = do(h, "GET", "/"+got.Code, nil)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://lekopien.com/projects" {
		t.Fatalf("redirect: got %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	rec = do(h, "GET", "/api/links/"+got.Code, nil)
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Clicks != 1 {
		t.Fatalf("clicks = %d, want 1", got.Clicks)
	}
}

func TestValidation(t *testing.T) {
	_, h := newServer(t)
	cases := []struct {
		name string
		body any
		want int
	}{
		{"not json", "nope", http.StatusBadRequest},
		{"relative url", map[string]any{"url": "/local"}, http.StatusUnprocessableEntity},
		{"javascript scheme", map[string]any{"url": "javascript:alert(1)"}, http.StatusUnprocessableEntity},
		{"bad alias", map[string]any{"url": "https://a.io", "alias": "a b"}, http.StatusUnprocessableEntity},
		{"reserved alias", map[string]any{"url": "https://a.io", "alias": "api"}, http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if rec := do(h, "POST", "/api/links", c.body); rec.Code != c.want {
				t.Fatalf("got %d, want %d (%s)", rec.Code, c.want, rec.Body)
			}
		})
	}
}

func TestAliasConflictAndDelete(t *testing.T) {
	_, h := newServer(t)
	body := map[string]any{"url": "https://a.io", "alias": "promo"}
	if rec := do(h, "POST", "/api/links", body); rec.Code != http.StatusCreated {
		t.Fatalf("first create: %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/links", body); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate alias: %d", rec.Code)
	}
	if rec := do(h, "DELETE", "/api/links/promo", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := do(h, "GET", "/promo", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("after delete: %d", rec.Code)
	}
}

func TestExpiry(t *testing.T) {
	s, h := newServer(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Store.now = func() time.Time { return now }
	do(h, "POST", "/api/links", map[string]any{"url": "https://a.io", "alias": "soon", "ttlSeconds": 60})
	if rec := do(h, "GET", "/soon", nil); rec.Code != http.StatusFound {
		t.Fatalf("before expiry: %d", rec.Code)
	}
	now = now.Add(2 * time.Minute)
	if rec := do(h, "GET", "/soon", nil); rec.Code != http.StatusGone {
		t.Fatalf("after expiry: %d", rec.Code)
	}
}

func TestConcurrentClicks(t *testing.T) {
	s, h := newServer(t)
	do(h, "POST", "/api/links", map[string]any{"url": "https://a.io", "alias": "hot"})
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); do(h, "GET", "/hot", nil) }()
	}
	wg.Wait()
	if l, _ := s.Store.Get("hot"); l.Clicks != 200 {
		t.Fatalf("clicks = %d, want 200", l.Clicks)
	}
}

func TestPersistence(t *testing.T) {
	file := filepath.Join(t.TempDir(), "links.json")
	st, _ := NewStore(file)
	if _, err := st.Create("https://a.io", "keep", 0); err != nil {
		t.Fatal(err)
	}
	again, err := NewStore(file)
	if err != nil {
		t.Fatal(err)
	}
	if l, err := again.Get("keep"); err != nil || l.URL != "https://a.io" {
		t.Fatalf("reloaded link = %+v, %v", l, err)
	}
}

func BenchmarkRandomCode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RandomCode(6)
	}
}
