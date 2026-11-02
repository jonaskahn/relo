// Claude upstream identity: stable conversation and device identifiers.
package account

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// AccountSessionID resolves the conversation id an account sends upstream.
// It is derived from the connection, the account, and the conversation, so
// one conversation keeps one id on an account across requests, retries, and
// restarts, a conversation that moves to another account is a different
// conversation there, and moving back finds the id the account already knew.
// An empty anchor names no conversation and yields no id.
func AccountSessionID(providerID, credentialID, anchor string) string {
	trimmed := strings.TrimSpace(anchor)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("relo-account-session-v1\x00" + providerID + "\x00" + credentialID + "\x00" + trimmed))
	return formatSessionUUID(sum)
}

// ClaudeDeviceID derives the stable device id one installation reports for
// an account. Token rotation never changes it; a different account on the
// same installation reports a different id.
func ClaudeDeviceID(seed, accountID string) string {
	sum := sha256.Sum256([]byte("relo-claude-device-id-v1\x00" + seed + "\x00" + accountID))
	return hex.EncodeToString(sum[:])
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
