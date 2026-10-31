package worker

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const jobIDBytes = 8

// newJobID returns a random identifier for log lines and error reports.
func newJobID() (string, error) {
	b := make([]byte, jobIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate job id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
