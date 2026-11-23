package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Note struct {
	NoteName   string    `json:"noteName"`
	NoteValue  string    `json:"noteValue"`
	Updated    time.Time `json:"updated"`
	Persistent bool      `json:"persistent"`
}

var (
	ErrNotFound     = errors.New("note not found")
	ErrValueMissing = errors.New("noteValue is required when creating a note")
)

// Store keeps notes in an ordered slice (insertion order is preserved in the
// JSON file) and rewrites the whole file atomically after every mutation.
type Store struct {
	mu    sync.Mutex
	path  string
	notes []Note
	now   func() time.Time // system clock; swappable in tests
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, now: func() time.Time { return time.Now().UTC() }}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.notes = []Note{}
		return s, nil
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal(b, &s.notes); err != nil {
		return nil, err
	}
	if s.notes == nil {
		s.notes = []Note{}
	}
	return s, nil
}

func (s *Store) indexOf(name string) int {
	for i := range s.notes {
		if s.notes[i].NoteName == name {
			return i
		}
	}
	return -1
}

// persist must be called with s.mu held.
func (s *Store) persist() error {
	b, err := json.MarshalIndent(s.notes, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".notes-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after successful rename
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

// Normalize drops every non-persistent note where now - updated > ttl.
// Called before every request.
func (s *Store) Normalize(ttl time.Duration) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	kept := s.notes[:0]
	var removed []string
	for _, n := range s.notes {
		if !n.Persistent && now.Sub(n.Updated) > ttl {
			removed = append(removed, n.NoteName)
			continue
		}
		kept = append(kept, n)
	}
	s.notes = kept
	if len(removed) == 0 {
		return nil, nil
	}
	return removed, s.persist()
}

func (s *Store) List() []Note {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Note, len(s.notes))
	copy(out, s.notes)
	return out
}

func (s *Store) Get(name string) (Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.indexOf(name); i >= 0 {
		return s.notes[i], nil
	}
	return Note{}, ErrNotFound
}

// Upsert creates or updates a note. Nil fields keep the existing value;
// on create, value is required and persistent defaults to false.
// `updated` is always set to the server's current time.
func (s *Store) Upsert(name string, value *string, persistent *bool) (Note, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	i := s.indexOf(name)
	created := i < 0
	if created {
		if value == nil {
			return Note{}, false, ErrValueMissing
		}
		s.notes = append(s.notes, Note{NoteName: name})
		i = len(s.notes) - 1
	}
	n := &s.notes[i]
	if value != nil {
		n.NoteValue = *value
	}
	if persistent != nil {
		n.Persistent = *persistent
	}
	n.Updated = s.now()
	return *n, created, s.persist()
}

func (s *Store) Drop(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.indexOf(name)
	if i < 0 {
		return ErrNotFound
	}
	s.notes = append(s.notes[:i], s.notes[i+1:]...)
	return s.persist()
}
