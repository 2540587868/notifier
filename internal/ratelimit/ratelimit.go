package ratelimit

import (
	"fmt"
	"sync"
	"time"

	"github.com/ysqss/notifier/internal/config"
	"github.com/ysqss/notifier/internal/message"
)

type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	config  config.RateLimitConfig
}

type bucket struct {
	timestamps []time.Time
}

func New(cfg config.RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*bucket),
		config:  cfg,
	}
}

func (rl *RateLimiter) Reject(msg *message.Message) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	source := msg.Tags["source"]

	key1 := fmt.Sprintf("%s:%s", source, string(msg.Level))
	if rl.isOverLimit(key1, rl.config.PerSourceLevel) {
		return true
	}

	key2 := source
	if key2 != "" {
		if rl.isOverLimit(key2, rl.config.PerSource) {
			return true
		}
	}

	key3 := "_global"
	if rl.isOverLimit(key3, rl.config.Global) {
		return true
	}

	rl.record(key1)
	if key2 != "" {
		rl.record(key2)
	}
	rl.record(key3)

	return false
}

func (rl *RateLimiter) isOverLimit(key string, rule config.LimitRule) bool {
	if rule.Max <= 0 {
		return false
	}

	b, ok := rl.buckets[key]
	if !ok {
		return false
	}

	window := rule.ParseWindow()
	cutoff := time.Now().Add(-window)
	count := 0
	for _, t := range b.timestamps {
		if t.After(cutoff) {
			count++
		}
	}
	return count >= rule.Max
}

func (rl *RateLimiter) record(key string) {
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{}
		rl.buckets[key] = b
	}
	b.timestamps = append(b.timestamps, time.Now())

	if len(b.timestamps) > 1000 {
		window := 10 * time.Minute
		cutoff := time.Now().Add(-window)
		filtered := b.timestamps[:0]
		for _, t := range b.timestamps {
			if t.After(cutoff) {
				filtered = append(filtered, t)
			}
		}
		b.timestamps = filtered
	}
}
