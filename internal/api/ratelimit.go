package api

import (
	"sync"
	"time"
)

type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientData
	limit   int
	window  time.Duration
}

type clientData struct {
	count     int
	lastError time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		clients: make(map[string]*clientData),
		limit:   limit,
		window:  window,
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	data, exists := rl.clients[ip]
	if !exists {
		return true
	}
	if time.Since(data.lastError) > rl.window {
		// reset
		data.count = 0
		return true
	}
	return data.count < rl.limit
}

func (rl *rateLimiter) recordFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	data, exists := rl.clients[ip]
	if !exists {
		data = &clientData{}
		rl.clients[ip] = data
	}
	data.count++
	data.lastError = time.Now()
}

func (rl *rateLimiter) reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.clients, ip)
}
