package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestAPI(t *testing.T) (*API, *Store, *time.Time) {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "notes.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	return &API{store: s, token: []byte("test-token"), ttl: 2 * time.Hour}, s, &clock
}

func do(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestFlow(t *testing.T) {
	api, s, clock := newTestAPI(t)
	h := api.Routes()
	const tok = "test-token"

	if r := do(t, h, "GET", "/notes", "", ""); r.Code != 401 {
		t.Fatalf("no token: want 401 got %d", r.Code)
	}
	if r := do(t, h, "GET", "/notes", "", "wrong"); r.Code != 401 {
		t.Fatalf("bad token: want 401 got %d", r.Code)
	}

	// create without persistent -> defaults false
	r := do(t, h, "POST", "/notes", `{"noteName":"a","noteValue":"{\"x\":1}"}`, tok)
	if r.Code != 201 || !strings.Contains(r.Body.String(), `"persistent":false`) {
		t.Fatalf("create: %d %s", r.Code, r.Body)
	}
	// create without value -> 400
	if r := do(t, h, "POST", "/notes", `{"noteName":"b"}`, tok); r.Code != 400 {
		t.Fatalf("create w/o value: want 400 got %d", r.Code)
	}
	// create persistent
	if r := do(t, h, "POST", "/notes", `{"noteName":"p","noteValue":"{}","persistent":true}`, tok); r.Code != 201 {
		t.Fatalf("create p: %d", r.Code)
	}
	// update value only -> keeps persistent=true
	r = do(t, h, "POST", "/notes", `{"noteName":"p","noteValue":"v2"}`, tok)
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"persistent":true`) || !strings.Contains(r.Body.String(), "v2") {
		t.Fatalf("update: %d %s", r.Code, r.Body)
	}

	// exactly at ttl: not expired (strictly greater)
	*clock = clock.Add(2 * time.Hour)
	if r := do(t, h, "GET", "/notes/a", "", tok); r.Code != 200 {
		t.Fatalf("at ttl should survive: %d", r.Code)
	}
	// past ttl: a expires, p (persistent) stays
	*clock = clock.Add(time.Second)
	if r := do(t, h, "GET", "/notes/a", "", tok); r.Code != 404 {
		t.Fatalf("expired: want 404 got %d", r.Code)
	}
	if r := do(t, h, "GET", "/notes/p", "", tok); r.Code != 200 {
		t.Fatalf("persistent should survive: %d", r.Code)
	}

	s2, err := OpenStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.List(); len(got) != 1 || got[0].NoteName != "p" {
		t.Fatalf("reload: %+v", got)
	}

	if r := do(t, h, "DELETE", "/notes/p", "", tok); r.Code != 204 {
		t.Fatalf("drop: %d", r.Code)
	}
	if r := do(t, h, "DELETE", "/notes/p", "", tok); r.Code != 404 {
		t.Fatalf("drop missing: %d", r.Code)
	}
	if r := do(t, h, "GET", "/notes", "", tok); strings.TrimSpace(r.Body.String()) != "[]" {
		t.Fatalf("empty list should be []: %s", r.Body)
	}
}
