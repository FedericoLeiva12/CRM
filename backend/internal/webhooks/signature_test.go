package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func TestSignCoversTimestampAndBody(t *testing.T) {
	body := []byte(`{"ok":true}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte("1700000000."))
	_, _ = mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if Sign("secret", 1700000000, body) != expected {
		t.Fatal("signature does not cover timestamp and raw body")
	}
	if Sign("secret", 1700000001, body) == expected {
		t.Fatal("timestamp was not included")
	}
}

func TestBackoff(t *testing.T) {
	if Backoff(1, time.Second, time.Minute) != time.Second {
		t.Fatal("first retry")
	}
	if Backoff(2, time.Second, time.Minute) != 2*time.Second {
		t.Fatal("second retry")
	}
	if Backoff(3, time.Second, time.Minute) != 4*time.Second {
		t.Fatal("third retry")
	}
	if Backoff(20, time.Second, 5*time.Second) != 5*time.Second {
		t.Fatal("backoff was not capped")
	}
}
