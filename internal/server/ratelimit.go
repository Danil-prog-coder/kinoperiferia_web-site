package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rateLimiter — счётчик запросов на клиента в скользящем окне. Хранится в
// памяти: заявок с сайта немного, отдельное хранилище под них избыточно.
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
	now    func() time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		hits:   map[string][]time.Time{},
		limit:  limit,
		window: window,
		now:    time.Now,
	}
}

// allow разрешает запрос и учитывает его, если лимит ещё не исчерпан.
func (l *rateLimiter) allow(key string) bool {
	if l == nil || l.limit <= 0 {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	cutoff := now.Add(-l.window)

	// Карта растёт на каждом новом адресе, поэтому её нужно чистить. Полный
	// обход на каждом запросе стоил бы O(числа адресов), так что подметаем
	// только когда накопилось заметно много ключей.
	if len(l.hits) > sweepThreshold {
		for k, times := range l.hits {
			if kept := prune(times, cutoff); len(kept) == 0 {
				delete(l.hits, k)
			} else {
				l.hits[k] = kept
			}
		}
	}

	times := prune(l.hits[key], cutoff)
	if len(times) >= l.limit {
		l.hits[key] = times
		return false
	}
	l.hits[key] = append(times, now)
	return true
}

// sweepThreshold — с какого числа отслеживаемых адресов включается уборка.
const sweepThreshold = 256

// prune оставляет только отметки внутри окна. Результат пишется поверх
// исходного среза: новые отметки всегда добавляются в хвост, поэтому
// переиспользование памяти безопасно.
func prune(times []time.Time, cutoff time.Time) []time.Time {
	kept := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	return kept
}

// clientIP определяет адрес клиента. X-Forwarded-For учитывается, потому что
// сайт рассчитан на работу за обратным прокси; при прямом доступе заголовка нет.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if first, _, ok := strings.Cut(fwd, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(fwd)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
