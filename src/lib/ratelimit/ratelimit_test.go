package ratelimit

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }
func newLimiter(max int, window time.Duration) (*Limiter, *clock) {
	c := &clock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := New(max, window)
	l.SetClock(c.now)
	return l, c
}

func TestBlocksAtMaxAndExpires(t *testing.T) {
	l, c := newLimiter(3, 15*time.Minute)
	for i := 0; i < 3; i++ {
		if blocked, _ := l.Blocked("a"); blocked {
			t.Fatalf("blocked after %d failures", i)
		}
		l.Fail("a")
		c.add(time.Minute)
	}
	blocked, retry := l.Blocked("a")
	if !blocked || retry != 12*time.Minute {
		t.Fatalf("blocked=%v retry=%v", blocked, retry)
	}
	if blocked, _ := l.Blocked("b"); blocked {
		t.Fatal("independent key blocked")
	}
	c.add(12 * time.Minute)
	if blocked, _ := l.Blocked("a"); blocked {
		t.Fatal("still blocked after window")
	}
	// A new window starts on the next failure.
	l.Fail("a")
	if blocked, _ := l.Blocked("a"); blocked {
		t.Fatal("expired failures carried into new window")
	}
}

func TestReset(t *testing.T) {
	l, _ := newLimiter(2, time.Minute)
	l.Fail("a")
	l.Fail("a")
	if blocked, _ := l.Blocked("a"); !blocked {
		t.Fatal("not blocked")
	}
	l.Reset("a")
	if blocked, _ := l.Blocked("a"); blocked {
		t.Fatal("blocked after reset")
	}
}

func TestMaxKeys(t *testing.T) {
	l, c := newLimiter(1, 10*time.Minute)
	l.MaxKeys = 3
	for i := 0; i < 3; i++ {
		l.Fail(fmt.Sprint(i))
		c.add(time.Minute)
	}
	// Full: the entry with the earliest reset ("0") is dropped.
	l.Fail("3")
	if l.Len() != 3 {
		t.Fatalf("len=%d", l.Len())
	}
	if blocked, _ := l.Blocked("0"); blocked {
		t.Fatal("oldest key kept")
	}
	for _, k := range []string{"1", "2", "3"} {
		if blocked, _ := l.Blocked(k); !blocked {
			t.Fatalf("key %s evicted", k)
		}
	}
	// Expired entries are pruned before evicting live ones.
	c.add(8*time.Minute + 30*time.Second) // "1" (reset at 11m) has expired
	l.Fail("4")
	if l.Len() != 3 {
		t.Fatalf("len=%d", l.Len())
	}
	for _, k := range []string{"2", "3", "4"} {
		if blocked, _ := l.Blocked(k); !blocked {
			t.Fatalf("key %s evicted", k)
		}
	}
	// Existing keys never trigger eviction.
	l.Fail("4")
	if l.Len() != 3 {
		t.Fatalf("len=%d", l.Len())
	}
}

func TestConcurrent(t *testing.T) {
	l := New(1000, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				l.Fail("k")
				l.Blocked("k")
			}
		}()
	}
	wg.Wait()
	if l.entries["k"].count != 500 {
		t.Fatalf("count=%d", l.entries["k"].count)
	}
}
