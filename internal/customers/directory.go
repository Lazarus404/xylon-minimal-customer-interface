package customers

import (
	"sync"
)

// Status values for a customer account.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
)

// Customer is the in-memory source-of-truth record (no secrets).
type Customer struct {
	ID       string         `json:"customer_id"`
	Status   string         `json:"status"`
	Features map[string]any `json:"features"`
	Notes    string         `json:"notes,omitempty"`
}

// Directory is a thread-safe in-memory customer map.
type Directory struct {
	mu   sync.RWMutex
	byID map[string]*Customer
}

// New returns an empty directory.
func New() *Directory {
	return &Directory{byID: make(map[string]*Customer)}
}

// Upsert inserts or replaces a customer.
func (d *Directory) Upsert(c Customer) {
	d.mu.Lock()
	defer d.mu.Unlock()
	cp := c
	if cp.Features == nil {
		cp.Features = map[string]any{}
	}
	d.byID[c.ID] = &cp
}

// Get returns a copy of the customer or false if missing.
func (d *Directory) Get(id string) (Customer, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	c, ok := d.byID[id]
	if !ok {
		return Customer{}, false
	}
	return clone(*c), true
}

// List returns all customers sorted by id.
func (d *Directory) List() []Customer {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Customer, 0, len(d.byID))
	for _, c := range d.byID {
		out = append(out, clone(*c))
	}
	// small n; insertion order not required -- sort for stable API
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].ID < out[i].ID {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// SetStatus updates status for an existing customer.
func (d *Directory) SetStatus(id, status string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	c, ok := d.byID[id]
	if !ok {
		return false
	}
	c.Status = status
	return true
}

func clone(c Customer) Customer {
	feats := map[string]any{}
	for k, v := range c.Features {
		feats[k] = v
	}
	c.Features = feats
	return c
}
