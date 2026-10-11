// Claude upstream identity: stable conversation and device identifiers.
package account

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// ClaudeSessionID resolves the conversation id a Claude request sends. A
// native UUID passes through unchanged; any other anchor maps
// deterministically into UUID shape, so the same client conversation keeps
// the same provider session across requests, retries, and restarts.
func ClaudeSessionID(namespace, anchor string) string {
	trimmed := strings.TrimSpace(anchor)
	if isUUID(trimmed) {
		return trimmed
	}
	sum := sha256.Sum256([]byte("relo-claude-session-v1\x00" + namespace + "\x00" + trimmed))
	return formatSessionUUID(sum)
}

// ClaudeDeviceID derives the stable device id one installation reports for
// an account. Token rotation never changes it; a different account on the
// same installation reports a different id.
func ClaudeDeviceID(seed, accountID string) string {
	sum := sha256.Sum256([]byte("relo-claude-device-id-v1\x00" + seed + "\x00" + accountID))
	return hex.EncodeToString(sum[:])
}

func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		switch {
		case index == 8 || index == 13 || index == 18 || index == 23:
			if char != '-' {
				return false
			}
		case char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F':
		default:
			return false
		}
	}
	return true
}

func formatSessionUUID(raw [sha256.Size]byte) string {
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, 36)
	for index, value := range raw[:16] {
		if index == 4 || index == 6 || index == 8 || index == 10 {
			out = append(out, '-')
		}
		out = append(out, hexDigits[value>>4], hexDigits[value&0x0f])
	}
	return string(out)
}
