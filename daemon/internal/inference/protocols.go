package inference

// The client protocols the data plane speaks, one per listener. The
// identifier names the port in the configuration, keys the address in the
// status API, and selects the client a profile is written for.
const (
	ProtocolOpenAI    = "openai"
	ProtocolAnthropic = "anthropic"
	ProtocolGemini    = "gemini"
)

// DataPlaneProtocols lists every protocol a data plane port can carry, in
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
