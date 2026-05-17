package dedup

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/ysqss/notifier/internal/message"
)

type Deduplicator struct {
	mu    sync.Mutex
	cache map[string]time.Time
	ttl   time.Duration
	max   int
}

func New(ttl time.Duration, maxSize int) *Deduplicator {
	return &Deduplicator{
		cache: make(map[string]time.Time, maxSize),
		ttl:   ttl,
		max:   maxSize,
	}
}

func (d *Deduplicator) IsDuplicate(msg *message.Message) bool {
	key := d.hash(msg)

	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	if t, ok := d.cache[key]; ok {
		if now.Sub(t) < d.ttl {
			return true
		}
		delete(d.cache, key)
	}

	d.cache[key] = now

	if len(d.cache) > d.max {
		d.evict()
	}

	return false
}

func (d *Deduplicator) hash(msg *message.Message) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s", msg.Title, msg.Content, msg.Level)))
	return fmt.Sprintf("%x", h[:8])
}

func (d *Deduplicator) evict() {
	cutoff := time.Now().Add(-d.ttl)
	for k, t := range d.cache {
		if t.Before(cutoff) {
			delete(d.cache, k)
		}
	}
}
