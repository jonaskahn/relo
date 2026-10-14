package inference

// The client protocols the data plane speaks, one per prefix. The
// identifier keys the address in the status API and selects the client a
// profile is written for.
const (
	ProtocolOpenAI    = "openai"
	ProtocolAnthropic = "anthropic"
	ProtocolGemini    = "gemini"
)

// DataPlaneProtocols lists every protocol the data plane can carry, in
// the order the surfaces that list them use.
func DataPlaneProtocols() []string {
	return []string{ProtocolOpenAI, ProtocolAnthropic, ProtocolGemini}
}

// ProtocolKnown reports whether a protocol identifier is one Relo serves.
func ProtocolKnown(protocol string) bool {
	for _, known := range DataPlaneProtocols() {
		if known == protocol {
			return true
		}
	}
	return false
}

// DataPlanePath returns the path prefix one protocol is served under on the
// single listener, and an empty string for a protocol with no surfaces.
func DataPlanePath(protocol string) string {
	switch protocol {
	case ProtocolOpenAI:
		return "/inference"
	case ProtocolAnthropic:
		return "/inference-claude"
	default:
		return ""
	}
}
