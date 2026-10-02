package auth

import (
	"sync"
	"time"
)

const (
	loginWindow      = 15 * time.Minute
	maxLoginAttempts = 10
	maxTrackedEmails = 10000
)

type loginLimiter struct {
	mutex    sync.Mutex
	attempts map[string][]time.Time
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{attempts: make(map[string][]time.Time)} }
func (limiter *loginLimiter) allow(email string) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := time.Now()
	recent := []time.Time{}
	for _, attempt := range limiter.attempts[email] {
		if now.Sub(attempt) < loginWindow {
			recent = append(recent, attempt)
		}
	}
	if len(recent) >= maxLoginAttempts {
		return false
	}
	if len(limiter.attempts) >= maxTrackedEmails {
		for key, attempts := range limiter.attempts {
			if len(attempts) == 0 || now.Sub(attempts[len(attempts)-1]) >= loginWindow {
				delete(limiter.attempts, key)
			}
		}
		if _, tracked := limiter.attempts[email]; !tracked && len(limiter.attempts) >= maxTrackedEmails {
			return false
		}
	}
	limiter.attempts[email] = append(recent, now)
	return true
}
func (limiter *loginLimiter) reset(email string) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	delete(limiter.attempts, email)
}
