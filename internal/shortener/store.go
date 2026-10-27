// Package shortener implements a URL shortener: code generation, storage and HTTP handlers.
package shortener

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"sync"
	"time"
)

const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

var (
	ErrNotFound = errors.New("link not found")
	ErrTaken    = errors.New("alias already taken")
	ErrExpired  = errors.New("link expired")
)

// Link is one shortened URL.
type Link struct {
	Code      string     `json:"code"`
	URL       string     `json:"url"`
	Clicks    int64      `json:"clicks"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// Store is a concurrency-safe map of links, optionally persisted to a JSON file.
type Store struct {
	mu    sync.RWMutex
	links map[string]*Link
	file  string
	now   func() time.Time
}

// NewStore loads links from file (if it exists). An empty file name keeps everything in memory.
func NewStore(file string) (*Store, error) {
	s := &Store{links: map[string]*Link{}, file: file, now: time.Now}
	if file == "" {
		return s, nil
	}
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*Link
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	for _, l := range list {
		s.links[l.Code] = l
	}
	return s, nil
}

// RandomCode returns a cryptographically random base62 string.
func RandomCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[idx.Int64()]
	}
	return string(b)
}

// Create stores url under alias, or under a fresh random code when alias is empty.
func (s *Store) Create(url, alias string, ttl time.Duration) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code := alias
	if code == "" {
		for code = RandomCode(6); s.links[code] != nil; code = RandomCode(6) {
		}
	} else if s.links[code] != nil {
		return nil, ErrTaken
	}
	l := &Link{Code: code, URL: url, CreatedAt: s.now().UTC()}
	if ttl > 0 {
		exp := l.CreatedAt.Add(ttl)
		l.ExpiresAt = &exp
	}
	s.links[code] = l
	return l, s.saveLocked()
}

// Resolve returns the link for code and counts the visit.
func (s *Store) Resolve(code string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.links[code]
	if l == nil {
		return nil, ErrNotFound
	}
	if l.ExpiresAt != nil && s.now().After(*l.ExpiresAt) {
		return nil, ErrExpired
	}
	l.Clicks++
	cp := *l
	return &cp, s.saveLocked()
}

// Get returns a copy of the link without counting a visit.
func (s *Store) Get(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l := s.links[code]
	if l == nil {
		return nil, ErrNotFound
	}
	cp := *l
	return &cp, nil
}

// Delete removes a link.
func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.links[code] == nil {
		return ErrNotFound
	}
	delete(s.links, code)
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if s.file == "" {
		return nil
	}
	list := make([]*Link, 0, len(s.links))
	for _, l := range s.links {
		list = append(list, l)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.file)
}
