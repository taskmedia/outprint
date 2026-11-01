// Package admission decides whether a render may start. A global slot limit
// with a bounded queue keeps the Workers from being swamped, and a per-user
// limit keeps one user from taking every slot. A request that does not fit is
// rejected at once, with a hint how long to wait; nothing is ever dropped
// silently or truncated.
//
// The per-user key is whatever the caller passes. The Gateway passes the user
// id that Outline confirmed for the session, never a value the client chose.
package admission

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/taskmedia/outprint/internal/limits"
)

// Reasons for a rejection. Test with errors.Is.
var (
	// ErrBusy: every slot is taken and the queue is full, or a queued
	// request waited for MaxWait without getting a slot.
	ErrBusy = errors.New("admission: busy")
	// ErrUserLimit: the user already has the allowed number of renders,
	// queued and running together.
	ErrUserLimit = errors.New("admission: user limit reached")
	// ErrNoUser: there is no user to count the request against.
	ErrNoUser = errors.New("admission: no user id")
)

// Rejected is a refused request. RetryAfter is in whole seconds, at least 1.
type Rejected struct {
	Reason     error
	RetryAfter int
}

func (r *Rejected) Error() string { return r.Reason.Error() }

func (r *Rejected) Unwrap() error { return r.Reason }

// Config sizes the controller. The zero value is not valid; start from
// Defaults.
type Config struct {
	// Slots is how many renders run at the same time (the number of Worker
	// replicas, as each Worker runs one job).
	Slots int `yaml:"slots"`
	// QueueDepth is how many requests may wait for a slot. 0 means none wait.
	QueueDepth int `yaml:"queueDepth"`
	// PerUser is how many renders one user may have, queued and running.
	PerUser int `yaml:"perUser"`
	// MaxWait is how long a queued request waits for a slot.
	MaxWait time.Duration `yaml:"maxWait"`
	// RenderTimeout caps the Retry-After estimate; it is the render time
	// limit of the Worker and is not read from the file.
	RenderTimeout time.Duration `yaml:"-"`
}

// Defaults returns the shipped configuration: one slot, a queue of four,
// one render per user.
func Defaults() Config {
	return Config{
		Slots:      limits.AdmissionSlots,
		QueueDepth: limits.AdmissionQueueDepth,
		PerUser:    limits.AdmissionPerUser,
		MaxWait:    limits.AdmissionMaxWait,
	}
}

// Validate reports every invalid field.
func (c Config) Validate() error {
	var errs []error
	if c.Slots < 1 {
		errs = append(errs, fmt.Errorf("slots must be a positive integer, got %d", c.Slots))
	}
	if c.QueueDepth < 0 {
		errs = append(errs, fmt.Errorf("queueDepth must be zero or a positive integer, got %d", c.QueueDepth))
	}
	if c.PerUser < 1 {
		errs = append(errs, fmt.Errorf("perUser must be a positive integer, got %d", c.PerUser))
	}
	if c.MaxWait <= 0 {
		errs = append(errs, fmt.Errorf("maxWait must be a positive duration, got %s", c.MaxWait))
	}
	return errors.Join(errs...)
}

type waiter struct {
	granted chan struct{}
}

// Controller is safe for concurrent use.
type Controller struct {
	cfg Config
	now func() time.Time

	mu      sync.Mutex
	running int
	queue   []*waiter
	users   map[string]int
	avgHold time.Duration
}

// New returns a controller, or the problems of an invalid configuration.
func New(cfg Config) (*Controller, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("admission: %w", err)
	}
	seed := limits.AdmissionAssumedRender
	if cfg.RenderTimeout > 0 && cfg.RenderTimeout < seed {
		seed = cfg.RenderTimeout
	}
	return &Controller{cfg: cfg, now: time.Now, users: map[string]int{}, avgHold: seed}, nil
}

// MaxWait is how long a queued request waits at most.
func (c *Controller) MaxWait() time.Duration { return c.cfg.MaxWait }

// Stats is a snapshot, for logs and tests.
type Stats struct {
	Running, Queued, Users int
}

// Stats returns the current load.
func (c *Controller) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Stats{Running: c.running, Queued: len(c.queue), Users: len(c.users)}
}

// EnterUser counts one render against the user. It fails with a *Rejected
// (ErrUserLimit) when the user is at the limit. Call the returned function
// when the request is over, on every path.
func (c *Controller) EnterUser(userID string) (func(), error) {
	if userID == "" {
		return nil, ErrNoUser
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.users[userID] >= c.cfg.PerUser {
		return nil, &Rejected{Reason: ErrUserLimit, RetryAfter: c.retryAfterLocked()}
	}
	c.users[userID]++
	return once(func() { c.leaveUser(userID) }), nil
}

func (c *Controller) leaveUser(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.users[userID]--; c.users[userID] <= 0 {
		delete(c.users, userID)
	}
}

// AcquireSlot returns when the caller may render, or fails: with a *Rejected
// (ErrBusy) when the queue is full or MaxWait passed, with ctx's error when
// the caller gave up. Call the returned function when the render is over, on
// every path.
func (c *Controller) AcquireSlot(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	w, err := c.takeOrQueue()
	if err != nil {
		return nil, err
	}
	if w == nil {
		return c.slotReleaser(), nil
	}
	return c.waitFor(ctx, w)
}

// takeOrQueue takes a free slot (nil waiter), joins the queue, or rejects.
func (c *Controller) takeOrQueue() (*waiter, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running < c.cfg.Slots && len(c.queue) == 0 {
		c.running++
		return nil, nil
	}
	if len(c.queue) >= c.cfg.QueueDepth {
		return nil, &Rejected{Reason: ErrBusy, RetryAfter: c.retryAfterLocked()}
	}
	w := &waiter{granted: make(chan struct{})}
	c.queue = append(c.queue, w)
	return w, nil
}

func (c *Controller) waitFor(ctx context.Context, w *waiter) (func(), error) {
	timer := time.NewTimer(c.cfg.MaxWait)
	defer timer.Stop()
	var cause error
	select {
	case <-w.granted:
		return c.slotReleaser(), nil
	case <-ctx.Done():
		cause = ctx.Err()
	case <-timer.C:
		cause = &Rejected{Reason: ErrBusy}
	}
	c.mu.Lock()
	removed := c.removeLocked(w)
	retry := c.retryAfterLocked()
	c.mu.Unlock()
	if !removed {
		// The slot was handed over while we gave up: pass it on.
		c.freeSlot()
	}
	var rej *Rejected
	if errors.As(cause, &rej) {
		rej.RetryAfter = retry
	}
	return nil, cause
}

func (c *Controller) removeLocked(w *waiter) bool {
	for i, q := range c.queue {
		if q == w {
			c.queue = append(c.queue[:i:i], c.queue[i+1:]...)
			return true
		}
	}
	return false
}

func (c *Controller) slotReleaser() func() {
	start := c.now()
	return once(func() {
		c.recordHold(c.now().Sub(start))
		c.freeSlot()
	})
}

// freeSlot hands the slot to the longest waiter, or frees it.
func (c *Controller) freeSlot() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.queue) == 0 {
		c.running--
		return
	}
	next := c.queue[0]
	c.queue = c.queue[1:]
	close(next.granted)
}

func (c *Controller) recordHold(d time.Duration) {
	const weight = 0.3
	c.mu.Lock()
	defer c.mu.Unlock()
	c.avgHold = time.Duration(weight*float64(d) + (1-weight)*float64(c.avgHold))
}

// retryAfterLocked estimates when a place frees up: about one render, from
// the moving average of the renders so far, at least a second and at most
// the render time limit.
func (c *Controller) retryAfterLocked() int {
	seconds := int(math.Ceil(c.avgHold.Round(time.Millisecond).Seconds()))
	if limit := int(math.Ceil(c.cfg.RenderTimeout.Seconds())); c.cfg.RenderTimeout > 0 && seconds > limit {
		seconds = limit
	}
	return max(seconds, 1)
}

func once(f func()) func() {
	var o sync.Once
	return func() { o.Do(f) }
}
