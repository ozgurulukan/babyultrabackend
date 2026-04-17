package middleware

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/bubsiebackend/internal/model"
)

type rateLimitEntry struct {
	count     int
	resetAt   time.Time
}

type RateLimiter struct {
	mu      sync.RWMutex
	entries map[string]*rateLimitEntry
	max     int
	window  time.Duration
}

func NewRateLimiter(max int, windowSeconds int) *RateLimiter {
	rl := &RateLimiter{
		entries: make(map[string]*rateLimitEntry),
		max:     max,
		window:  time.Duration(windowSeconds) * time.Second,
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, entry := range rl.entries {
			if now.After(entry.resetAt) {
				delete(rl.entries, key)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := resolveKey(c)

		rl.mu.Lock()
		entry, exists := rl.entries[key]
		now := time.Now()

		if !exists || now.After(entry.resetAt) {
			rl.entries[key] = &rateLimitEntry{
				count:   1,
				resetAt: now.Add(rl.window),
			}
			rl.mu.Unlock()

			c.Set("X-RateLimit-Limit", itoa(rl.max))
			c.Set("X-RateLimit-Remaining", itoa(rl.max-1))
			return c.Next()
		}

		if entry.count >= rl.max {
			retryAfter := entry.resetAt.Sub(now).Seconds()
			rl.mu.Unlock()

			c.Set("X-RateLimit-Limit", itoa(rl.max))
			c.Set("X-RateLimit-Remaining", "0")
			c.Set("Retry-After", itoa(int(retryAfter)+1))

			return model.ErrorResponse(c, fiber.StatusTooManyRequests, "rate limit exceeded, try again later")
		}

		entry.count++
		remaining := rl.max - entry.count
		rl.mu.Unlock()

		c.Set("X-RateLimit-Limit", itoa(rl.max))
		c.Set("X-RateLimit-Remaining", itoa(remaining))
		return c.Next()
	}
}

func resolveKey(c *fiber.Ctx) string {
	if uid, ok := c.Locals("uid").(string); ok && uid != "" {
		return "uid:" + uid
	}
	return "ip:" + c.IP()
}

func itoa(n int) string {
	if n < 0 {
		n = 0
	}
	s := ""
	if n == 0 {
		return "0"
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
