package admission

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func newController(t *testing.T, mutate func(*Config)) *Controller {
	t.Helper()
	cfg := Defaults()
	cfg.RenderTimeout = 60 * time.Second
	if mutate != nil {
		mutate(&cfg)
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mustAcquire(t *testing.T, c *Controller) func() {
	t.Helper()
	release, err := c.AcquireSlot(context.Background())
	if err != nil {
		t.Fatalf("AcquireSlot: %v", err)
	}
	return release
}

// waitFor polls until the condition holds; the queue is internal state, so
// a test cannot block on it.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

type acquireResult struct {
	release func()
	err     error
}

func acquireAsync(c *Controller, ctx context.Context) <-chan acquireResult {
	ch := make(chan acquireResult, 1)
	go func() {
		release, err := c.AcquireSlot(ctx)
		ch <- acquireResult{release, err}
	}()
	return ch
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		ok     bool
	}{
		{"defaults", func(*Config) {}, true},
		{"no queue is allowed", func(c *Config) { c.QueueDepth = 0 }, true},
		{"zero slots", func(c *Config) { c.Slots = 0 }, false},
		{"negative queue", func(c *Config) { c.QueueDepth = -1 }, false},
		{"zero per user", func(c *Config) { c.PerUser = 0 }, false},
		{"zero wait", func(c *Config) { c.MaxWait = 0 }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			tt.mutate(&cfg)
			_, err := New(cfg)
			if (err == nil) != tt.ok {
				t.Errorf("New error = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestSlotIsFreeThenQueuedThenRejected(t *testing.T) {
	c := newController(t, func(c *Config) { c.QueueDepth = 2 })
	release := mustAcquire(t, c)

	q1 := acquireAsync(c, context.Background())
	waitFor(t, "first waiter", func() bool { return c.Stats().Queued == 1 })
	q2 := acquireAsync(c, context.Background())
	waitFor(t, "second waiter", func() bool { return c.Stats().Queued == 2 })

	_, err := c.AcquireSlot(context.Background())
	var rej *Rejected
	if !errors.As(err, &rej) || !errors.Is(err, ErrBusy) {
		t.Fatalf("err = %v, want busy rejection", err)
	}
	if rej.RetryAfter < 1 {
		t.Errorf("RetryAfter = %d, want at least 1", rej.RetryAfter)
	}

	release()
	r1 := <-q1
	if r1.err != nil {
		t.Fatalf("first waiter: %v", r1.err)
	}
	if got := c.Stats(); got.Running != 1 || got.Queued != 1 {
		t.Errorf("stats after hand-over = %+v", got)
	}
	r1.release()
	r2 := <-q2
	if r2.err != nil {
		t.Fatalf("second waiter: %v", r2.err)
	}
	r2.release()
	if got := c.Stats(); got != (Stats{}) {
		t.Errorf("stats at rest = %+v", got)
	}
}

func TestQueueIsFirstInFirstOut(t *testing.T) {
	c := newController(t, nil)
	release := mustAcquire(t, c)
	first := acquireAsync(c, context.Background())
	waitFor(t, "first waiter", func() bool { return c.Stats().Queued == 1 })
	second := acquireAsync(c, context.Background())
	waitFor(t, "second waiter", func() bool { return c.Stats().Queued == 2 })

	release()
	r := <-first
	select {
	case <-second:
		t.Fatal("the later waiter got the slot first")
	case <-time.After(20 * time.Millisecond):
	}
	r.release()
	(<-second).release()
}

func TestNoQueueRejectsWhenSlotsAreTaken(t *testing.T) {
	c := newController(t, func(c *Config) { c.QueueDepth = 0 })
	release := mustAcquire(t, c)
	defer release()
	if _, err := c.AcquireSlot(context.Background()); !errors.Is(err, ErrBusy) {
		t.Errorf("err = %v, want busy", err)
	}
}

func TestMultipleSlots(t *testing.T) {
	c := newController(t, func(c *Config) { c.Slots = 2; c.QueueDepth = 0 })
	a, b := mustAcquire(t, c), mustAcquire(t, c)
	if _, err := c.AcquireSlot(context.Background()); !errors.Is(err, ErrBusy) {
		t.Errorf("third acquire err = %v, want busy", err)
	}
	a()
	mustAcquire(t, c)()
	b()
}

func TestWaiterGivesUpOnCancelAndFreesItsPlace(t *testing.T) {
	c := newController(t, func(c *Config) { c.QueueDepth = 1 })
	release := mustAcquire(t, c)
	ctx, cancel := context.WithCancel(context.Background())
	q := acquireAsync(c, ctx)
	waitFor(t, "waiter", func() bool { return c.Stats().Queued == 1 })

	cancel()
	if r := <-q; !errors.Is(r.err, context.Canceled) {
		t.Fatalf("err = %v, want canceled", r.err)
	}
	if got := c.Stats(); got.Queued != 0 || got.Running != 1 {
		t.Errorf("stats = %+v", got)
	}
	release()
	if got := c.Stats(); got != (Stats{}) {
		t.Errorf("stats = %+v, the cancelled waiter must not hold the slot", got)
	}
}

func TestCancelledContextNeverQueues(t *testing.T) {
	c := newController(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.AcquireSlot(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
	if got := c.Stats(); got != (Stats{}) {
		t.Errorf("stats = %+v", got)
	}
}

func TestWaiterTimesOutAfterMaxWait(t *testing.T) {
	c := newController(t, func(c *Config) { c.MaxWait = 20 * time.Millisecond })
	release := mustAcquire(t, c)
	defer release()
	_, err := c.AcquireSlot(context.Background())
	var rej *Rejected
	if !errors.As(err, &rej) || !errors.Is(err, ErrBusy) || rej.RetryAfter < 1 {
		t.Fatalf("err = %v, want busy rejection with a retry hint", err)
	}
	if c.Stats().Queued != 0 {
		t.Error("the timed-out waiter is still queued")
	}
}

func TestReleaseIsIdempotent(t *testing.T) {
	c := newController(t, func(c *Config) { c.QueueDepth = 0 })
	release := mustAcquire(t, c)
	release()
	release()
	a := mustAcquire(t, c)
	if _, err := c.AcquireSlot(context.Background()); !errors.Is(err, ErrBusy) {
		t.Errorf("a double release freed a slot twice: %v", err)
	}
	a()
}

func TestPerUserLimitCountsPerUser(t *testing.T) {
	c := newController(t, nil)
	leaveA, err := c.EnterUser("user-a")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.EnterUser("user-a")
	var rej *Rejected
	if !errors.As(err, &rej) || !errors.Is(err, ErrUserLimit) || rej.RetryAfter < 1 {
		t.Fatalf("second render of user-a: %v", err)
	}
	leaveB, err := c.EnterUser("user-b")
	if err != nil {
		t.Fatalf("user-b must not be blocked by user-a: %v", err)
	}
	leaveB()
	leaveA()
	leaveA()
	if c.Stats().Users != 0 {
		t.Error("user entries leak")
	}
	if leave, err := c.EnterUser("user-a"); err != nil {
		t.Errorf("after leaving: %v", err)
	} else {
		leave()
	}
}

func TestPerUserLimitIsConfigurable(t *testing.T) {
	c := newController(t, func(c *Config) { c.PerUser = 2 })
	a, _ := c.EnterUser("u")
	b, err := c.EnterUser("u")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.EnterUser("u"); !errors.Is(err, ErrUserLimit) {
		t.Errorf("third: %v", err)
	}
	a()
	b()
}

func TestEmptyUserIsRefused(t *testing.T) {
	c := newController(t, nil)
	if _, err := c.EnterUser(""); !errors.Is(err, ErrNoUser) {
		t.Errorf("err = %v", err)
	}
}

func TestRetryAfterIsBoundedByTheRenderTimeout(t *testing.T) {
	c := newController(t, func(c *Config) { c.RenderTimeout = 3 * time.Second })
	c.recordHold(10 * time.Minute)
	c.mu.Lock()
	got := c.retryAfterLocked()
	c.mu.Unlock()
	if got != 3 {
		t.Errorf("retryAfter = %d, want 3", got)
	}
}

func TestRetryAfterFollowsObservedRenderTime(t *testing.T) {
	c := newController(t, func(c *Config) { c.QueueDepth = 0 })
	now := time.Unix(0, 0)
	c.now = func() time.Time { return now }
	for range 30 {
		release := mustAcquire(t, c)
		now = now.Add(2 * time.Second)
		release()
	}
	release := mustAcquire(t, c)
	defer release()
	_, err := c.AcquireSlot(context.Background())
	var rej *Rejected
	if !errors.As(err, &rej) || rej.RetryAfter != 2 {
		t.Errorf("err = %v, want RetryAfter 2", err)
	}
}

func TestSmallRenderTimeoutLowersTheSeed(t *testing.T) {
	c := newController(t, func(c *Config) { c.RenderTimeout = 2 * time.Second })
	if c.avgHold != 2*time.Second {
		t.Errorf("seed = %s", c.avgHold)
	}
}

func TestConcurrentUseKeepsTheLimits(t *testing.T) {
	c := newController(t, func(c *Config) { c.Slots = 3; c.QueueDepth = 50; c.PerUser = 50 })
	var mu sync.Mutex
	running, peak := 0, 0
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			leave, err := c.EnterUser("u")
			if err != nil {
				t.Errorf("EnterUser %d: %v", i, err)
				return
			}
			defer leave()
			release, err := c.AcquireSlot(context.Background())
			if err != nil {
				t.Errorf("AcquireSlot %d: %v", i, err)
				return
			}
			defer release()
			mu.Lock()
			running++
			peak = max(peak, running)
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			running--
			mu.Unlock()
		}()
	}
	wg.Wait()
	if peak > 3 {
		t.Errorf("peak concurrency %d exceeds the 3 slots", peak)
	}
	if got := c.Stats(); got != (Stats{}) {
		t.Errorf("stats at rest = %+v", got)
	}
}

func TestChurnWithCancellationLeaksNothing(t *testing.T) {
	c := newController(t, func(c *Config) { c.QueueDepth = 20; c.MaxWait = 5 * time.Millisecond })
	var wg sync.WaitGroup
	for i := range 60 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(i%7)*time.Millisecond)
			defer cancel()
			if release, err := c.AcquireSlot(ctx); err == nil {
				time.Sleep(time.Millisecond)
				release()
			}
		}()
	}
	wg.Wait()
	if got := c.Stats(); got != (Stats{}) {
		t.Errorf("stats at rest = %+v: a slot or a queue place leaked", got)
	}
}
