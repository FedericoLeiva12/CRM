package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

const (
	HeaderSignature      = "X-CRM-Signature"
	HeaderTimestamp      = "X-CRM-Timestamp"
	HeaderIdempotencyKey = "X-CRM-Idempotency-Key"
	HeaderEvent          = "X-CRM-Event"
	SignaturePrefix      = "sha256="
	UserAgent            = "SiraCRM-Webhooks/1"
)

// Sign is HMAC-SHA256 over "<unix seconds>.<raw body>", hex-encoded.
// The timestamp is included so a captured signature cannot be replayed with a fresh timestamp.
func Sign(secret string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	_, _ = mac.Write([]byte{'.'})
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
