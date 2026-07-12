package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func newPlatformTradeNo() string {
	return "GC" + randomHex(12)
}

func newCashierToken() string {
	// ≥128-bit CSPRNG (16 bytes = 128 bits) as hex = 32 chars
	return randomHex(16)
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		// fallback should never happen
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
