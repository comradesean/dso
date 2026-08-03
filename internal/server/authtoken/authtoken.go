// Package authtoken holds the short-lived mapping from a game-server auth token
// to the AES-CWC key negotiated during authentication. The auth service writes
// entries; the game service (UDP) reads them to bind an incoming session to its
// key. Entries expire after a fixed TTL unless refreshed.
package authtoken

import (
	"sync"
	"time"
)

// TTL is how long an auth token remains valid before first use
// (BuildConfig::AUTH_TICKET_TIMEOUT).
const TTL = 30 * time.Second

// Token is the 8-byte game-server auth token, handled as raw bytes so it is
// endianness-agnostic across platforms.
type Token [8]byte

type entry struct {
	key     []byte
	expires time.Time
}

// Registry is a concurrency-safe token -> CWC key store with expiry.
type Registry struct {
	mu   sync.Mutex
	m    map[Token]entry
	now  func() time.Time
	ttl  time.Duration
	stop chan struct{}
}

// NewRegistry creates an empty registry with the default TTL.
func NewRegistry() *Registry {
	return &Registry{m: make(map[Token]entry), now: time.Now, ttl: TTL}
}

// Add registers a token with its CWC key, valid for the TTL.
func (r *Registry) Add(tok Token, key []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := make([]byte, len(key))
	copy(k, key)
	r.m[tok] = entry{key: k, expires: r.now().Add(r.ttl)}
}

// Lookup returns the CWC key for a token if present and unexpired, and refreshes
// its expiry (a live session keeps its token alive).
func (r *Registry) Lookup(tok Token) ([]byte, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.m[tok]
	if !ok {
		return nil, false
	}
	if r.now().After(e.expires) {
		delete(r.m, tok)
		return nil, false
	}
	e.expires = r.now().Add(r.ttl)
	r.m[tok] = e
	return e.key, true
}

// prune removes expired entries.
func (r *Registry) prune() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	for tok, e := range r.m {
		if now.After(e.expires) {
			delete(r.m, tok)
		}
	}
}

// StartJanitor runs periodic pruning until stopped; call Stop to end it.
func (r *Registry) StartJanitor(interval time.Duration) {
	r.stop = make(chan struct{})
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				r.prune()
			case <-r.stop:
				return
			}
		}
	}()
}

// Stop ends the janitor goroutine if running.
func (r *Registry) Stop() {
	if r.stop != nil {
		close(r.stop)
		r.stop = nil
	}
}
