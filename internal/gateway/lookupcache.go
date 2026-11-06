package gateway

import (
	"crypto/sha256"
	"sync"
	"time"

	"github.com/taskmedia/outprint/internal/limits"
)

// maxLookupEntries bounds the cache; when full, expired entries are dropped
// and, if that frees nothing, new entries are not stored.
const maxLookupEntries = 4096

type lookupKey [sha256.Size]byte

type lookupEntry struct {
	language string
	denied   bool
	expires  time.Time
}

// lookupCache keeps what the status endpoint asked Outline for a short time:
// the language of a session and the fact that a session was refused. It is
// keyed by a hash of the cookie, so the cookie itself is never stored. The
// status answer is for button visibility only; a stale entry costs a label.
type lookupCache struct {
	now func() time.Time
	mu  sync.Mutex
	m   map[lookupKey]lookupEntry
}

func newLookupCache(now func() time.Time) *lookupCache {
	return &lookupCache{now: now, m: map[lookupKey]lookupEntry{}}
}

func (c *lookupCache) get(cookie string) (lookupEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := lookupKey(sha256.Sum256([]byte(cookie)))
	e, ok := c.m[k]
	if !ok || !c.now().Before(e.expires) {
		delete(c.m, k)
		return lookupEntry{}, false
	}
	return e, true
}

func (c *lookupCache) put(cookie string, e lookupEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= maxLookupEntries {
		now := c.now()
		for k, v := range c.m {
			if !now.Before(v.expires) {
				delete(c.m, k)
			}
		}
		if len(c.m) >= maxLookupEntries {
			return
		}
	}
	c.m[lookupKey(sha256.Sum256([]byte(cookie)))] = e
}

func (c *lookupCache) denied(cookie string) bool {
	e, ok := c.get(cookie)
	return ok && e.denied
}

func (c *lookupCache) deny(cookie string) {
	c.put(cookie, lookupEntry{denied: true, expires: c.now().Add(limits.StatusDeniedTTL)})
}

func (c *lookupCache) language(cookie string) (string, bool) {
	e, ok := c.get(cookie)
	if !ok || e.denied {
		return "", false
	}
	return e.language, true
}

func (c *lookupCache) setLanguage(cookie, lang string) {
	c.put(cookie, lookupEntry{language: lang, expires: c.now().Add(limits.StatusTTL)})
}
