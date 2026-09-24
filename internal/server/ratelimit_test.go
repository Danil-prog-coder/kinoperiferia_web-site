package server

import (
	"strconv"
	"testing"
	"time"
)

func TestRateLimiterBlocksAfterLimit(t *testing.T) {
	l := newRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.allow("ip") {
			t.Fatalf("запрос %d отклонён до исчерпания лимита", i+1)
		}
	}
	if l.allow("ip") {
		t.Error("лимит не сработал")
	}
}

func TestRateLimiterCountsPerKey(t *testing.T) {
	l := newRateLimiter(1, time.Minute)
	if !l.allow("первый") || !l.allow("второй") {
		t.Error("лимит одного адреса задел другой")
	}
}

func TestRateLimiterReleasesAfterWindow(t *testing.T) {
	now := time.Now()
	l := newRateLimiter(1, time.Minute)
	l.now = func() time.Time { return now }

	if !l.allow("ip") || l.allow("ip") {
		t.Fatal("лимит не сработал внутри окна")
	}

	now = now.Add(2 * time.Minute)
	if !l.allow("ip") {
		t.Error("после окна запрос всё ещё блокируется")
	}
}

func TestRateLimiterForgetsStaleKeys(t *testing.T) {
	now := time.Now()
	l := newRateLimiter(5, time.Minute)
	l.now = func() time.Time { return now }

	for i := 0; i <= sweepThreshold; i++ {
		l.allow("ip-" + strconv.Itoa(i))
	}
	if len(l.hits) <= sweepThreshold {
		t.Fatalf("до уборки в карте %d ключей", len(l.hits))
	}

	now = now.Add(2 * time.Minute)
	l.allow("свежий")

	if len(l.hits) != 1 {
		t.Errorf("после уборки осталось %d ключей, ожидался 1", len(l.hits))
	}
}

func TestRateLimiterWithoutLimitAllowsEverything(t *testing.T) {
	l := newRateLimiter(0, time.Minute)
	for i := 0; i < 100; i++ {
		if !l.allow("ip") {
			t.Fatal("нулевой лимит блокирует запросы")
		}
	}
	var nilLimiter *rateLimiter
	if !nilLimiter.allow("ip") {
		t.Error("nil-лимитер блокирует запросы")
	}
}
