package run

import (
	"crypto/rand"
	"time"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// NewRunID returns "<YYMMDD-HHMMSS>-<4 random base36 chars>", UTC.
func NewRunID(now time.Time) string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return now.UTC().Format("060102-150405") + "-" + string(b)
}
