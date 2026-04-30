// Package store is the dashboard playground's in-memory persistence
// layer. Replace with a real database in production code; this file
// exists only to keep the example self-contained.
//
// Build constraint: this file does not opt into the sveltego build
// tag because it lives under src/lib (a regular Go package) and is
// imported by the generated mirror tree at build time.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User is one row in the user table.
type User struct {
	ID       string
	Username string
	hash     []byte
}

// Item is one row in the CRUD items table.
type Item struct {
	ID        string
	Title     string
	Note      string
	OwnerID   string
	UpdatedAt time.Time
}

// Sample is one polling-chart datapoint.
type Sample struct {
	TS    time.Time
	Value int
}

// Store is the singleton state for the playground. The zero value is
// not usable; call New to seed a default admin and ten items.
type Store struct {
	mu       sync.RWMutex
	users    map[string]*User // keyed by ID
	byName   map[string]*User // keyed by Username
	sessions map[string]string
	items    map[string]*Item
	itemsSeq int
	metrics  []Sample
}

// New returns a Store seeded with one admin user (admin / password123)
// and ten items so the dashboard list has something to render.
func New() *Store {
	s := &Store{
		users:    map[string]*User{},
		byName:   map[string]*User{},
		sessions: map[string]string{},
		items:    map[string]*Item{},
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		panic(err)
	}
	admin := &User{ID: "u-admin", Username: "admin", hash: hash}
	s.users[admin.ID] = admin
	s.byName[admin.Username] = admin

	for i := 1; i <= 5; i++ {
		s.itemsSeq++
		id := "i-" + strconv.Itoa(s.itemsSeq)
		s.items[id] = &Item{
			ID:        id,
			Title:     "Sample item " + strconv.Itoa(i),
			Note:      "Edit me",
			OwnerID:   admin.ID,
			UpdatedAt: time.Now(),
		}
	}
	return s
}

// Verify returns the matching user when the bcrypt comparison succeeds.
// Username lookup is case-sensitive.
func (s *Store) Verify(username, password string) (*User, error) {
	s.mu.RLock()
	u := s.byName[username]
	s.mu.RUnlock()
	if u == nil {
		return nil, errors.New("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword(u.hash, []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}
	return u, nil
}

// IssueSession mints an opaque token mapped to userID and returns it.
func (s *Store) IssueSession(userID string) (string, error) {
	tok, err := randHex(24)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.sessions[tok] = userID
	s.mu.Unlock()
	return tok, nil
}

// Lookup returns the user keyed by token, or nil when absent / expired.
func (s *Store) Lookup(token string) *User {
	if token == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	uid, ok := s.sessions[token]
	if !ok {
		return nil
	}
	return s.users[uid]
}

// Revoke drops the session for token (no-op when absent).
func (s *Store) Revoke(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// List returns items owned by ownerID, sorted by ID for deterministic
// rendering.
func (s *Store) List(ownerID string) []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Item
	for _, it := range s.items {
		if it.OwnerID != ownerID {
			continue
		}
		out = append(out, *it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns the item by ID (when owned by ownerID), or nil.
func (s *Store) Get(ownerID, id string) *Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	it, ok := s.items[id]
	if !ok || it.OwnerID != ownerID {
		return nil
	}
	cp := *it
	return &cp
}

// Create inserts a new item and returns it.
func (s *Store) Create(ownerID, title, note string) Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.itemsSeq++
	id := "i-" + strconv.Itoa(s.itemsSeq)
	it := &Item{
		ID:        id,
		Title:     title,
		Note:      note,
		OwnerID:   ownerID,
		UpdatedAt: time.Now(),
	}
	s.items[id] = it
	return *it
}

// Update mutates the item identified by id when ownerID matches; returns
// the updated value or an error when the item does not exist or is
// owned by another user.
func (s *Store) Update(ownerID, id, title, note string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[id]
	if !ok || it.OwnerID != ownerID {
		return Item{}, errors.New("item not found")
	}
	it.Title = title
	it.Note = note
	it.UpdatedAt = time.Now()
	return *it, nil
}

// Delete removes the item identified by id when ownerID matches.
func (s *Store) Delete(ownerID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[id]
	if !ok || it.OwnerID != ownerID {
		return errors.New("item not found")
	}
	delete(s.items, id)
	return nil
}

// Metrics returns a deterministic synthetic time series for the polling
// chart. Each call extends the in-memory ring buffer by one sample so
// repeated polls show motion. Bound to 12 entries.
func (s *Store) Metrics() []Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Truncate(time.Second)
	// Synthetic value: bounded sin-like wave seeded on second-of-minute.
	v := 30 + (int(now.Unix())%60)*2
	s.metrics = append(s.metrics, Sample{TS: now, Value: v})
	if len(s.metrics) > 12 {
		s.metrics = s.metrics[len(s.metrics)-12:]
	}
	out := make([]Sample, len(s.metrics))
	copy(out, s.metrics)
	return out
}

// Default is the process-wide store. Tests construct their own via New.
var Default = New()

func randHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
