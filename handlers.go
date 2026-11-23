package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
)

const maxBody = 64 << 10 // 64 KiB is plenty for a sticky note

type API struct {
	store *Store
	token []byte
	ttl   time.Duration
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	notes := http.NewServeMux()
	notes.HandleFunc("GET /notes", a.listAll)
	notes.HandleFunc("GET /notes/{name}", a.listItem)
	notes.HandleFunc("POST /notes", a.upsert)
	notes.HandleFunc("DELETE /notes/{name}", a.drop)

	h := a.auth(a.normalize(notes))
	mux.Handle("/notes", h)
	mux.Handle("/notes/", h)
	return logRequests(mux)
}

// auth accepts "Authorization: Bearer <token>" or "X-Api-Key: <token>"
// (the latter is easier to set in Apple Shortcuts).
func (a *API) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-Api-Key")
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			got = strings.TrimPrefix(h, "Bearer ")
		}
		if subtle.ConstantTimeCompare([]byte(got), a.token) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="on-context"`)
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// normalize drops expired notes before every request is handled.
func (a *API) normalize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if removed, err := a.store.Normalize(a.ttl); err != nil {
			log.Printf("normalize: %v", err)
		} else if len(removed) > 0 {
			log.Printf("normalize: expired %v", removed)
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) listAll(w http.ResponseWriter, _ *http.Request) {
	writeContent(w, http.StatusOK, a.store.List())
}

func (a *API) listItem(w http.ResponseWriter, r *http.Request) {
	n, err := a.store.Get(r.PathValue("name"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeContent(w, http.StatusOK, n)
}

type upsertReq struct {
	NoteName   string  `json:"noteName"`
	NoteValue  *string `json:"noteValue"`  // required on create; omitted = keep
	Persistent *bool   `json:"persistent"` // optional; omitted = keep (false on create)
}

// POST /notes -> 201 created, 200 updated
func (a *API) upsert(w http.ResponseWriter, r *http.Request) {
	var req upsertReq
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if len(req.NoteName) == 0 || len(req.NoteName) > 128 || strings.ContainsRune(req.NoteName, '/') {
		writeErr(w, http.StatusBadRequest, "noteName must be 1-128 chars, no '/'")
		return
	}
	n, created, err := a.store.Upsert(req.NoteName, req.NoteValue, req.Persistent)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeContent(w, status, n)
}

func (a *API) drop(w http.ResponseWriter, r *http.Request) {
	n, err := a.store.Drop(r.PathValue("name"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeContent(w, http.StatusOK, n) // returns the removed note
}

// ---- helpers ----

// envelope is the shape of every JSON response:
// success -> {"content": <data>}, failure -> {"content": null, "error": "..."}
type envelope struct {
	Content any    `json:"content"`
	Error   string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, env envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(env)
}

func writeContent(w http.ResponseWriter, status int, v any) {
	writeJSON(w, status, envelope{Content: v})
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, envelope{Error: msg})
}

func writeStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrValueMissing):
		writeErr(w, http.StatusBadRequest, err.Error())
	default:
		log.Printf("store error: %v", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(c int) { s.status = c; s.ResponseWriter.WriteHeader(c) }

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if r.URL.Path != "/healthz" {
			log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Microsecond))
		}
	})
}
