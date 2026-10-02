package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestLoginLimiterExpiresAndResets(t *testing.T) {
	limiter := newLoginLimiter()
	email := "admin@example.test"
	for attempt := 0; attempt < maxLoginAttempts; attempt++ {
		if !limiter.allow(email) {
			t.Fatal("Blocked before the attempt limit")
		}
	}
	if limiter.allow(email) {
		t.Fatal("Allowed an attempt beyond the limit")
	}
	limiter.reset(email)
	if !limiter.allow(email) {
		t.Fatal("Successful-login reset did not clear the limit")
	}
	limiter.attempts[email] = []time.Time{time.Now().Add(-loginWindow - time.Second)}
	if !limiter.allow(email) {
		t.Fatal("Expired attempts were not cleared")
	}
}
func TestLoginLimiterCapsMemory(t *testing.T) {
	limiter := newLoginLimiter()
	for index := 0; index < maxTrackedEmails; index++ {
		limiter.attempts[fmt.Sprintf("%d@example.test", index)] = []time.Time{time.Now()}
	}
	if limiter.allow("untracked@example.test") {
		t.Fatal("Allowed the tracked-key limit to grow")
	}
	limiter.attempts["0@example.test"] = []time.Time{time.Now().Add(-loginWindow - time.Second)}
	if !limiter.allow("untracked@example.test") {
		t.Fatal("Expired keys did not free limiter capacity")
	}
}
