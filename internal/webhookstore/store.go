package webhookstore

import "sync"

// Store keeps the last N webhook payloads in memory (lab UX).
type Store struct {
	mu   sync.Mutex
	max  int
	list []map[string]any
}

// New builds a store with capacity max (default 32).
func New(max int) *Store {
	if max < 1 {
		max = 32
	}
	return &Store{max: max}
}

// Add prepends an event.
func (s *Store) Add(ev map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.list = append([]map[string]any{ev}, s.list...)
	if len(s.list) > s.max {
		s.list = s.list[:s.max]
	}
}

// Latest returns the newest event.
func (s *Store) Latest() (map[string]any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.list) == 0 {
		return nil, false
	}
	return s.list[0], true
}
