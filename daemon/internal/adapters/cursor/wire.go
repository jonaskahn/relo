// Cursor agent endpoints, headers, and bounds.
package cursor

// Wire identity Relo announces to Cursor. The version stays pinned to the
// reference behavior this client was verified against.
const (
	DefaultBaseURL = "https://api2.cursor.sh"
	ClientVersion  = "cli-2026.09.02-c22c1a3"

	RunPath              = "/agent.v1.AgentService/Run"
	GetUsableModelsPath  = "/agent.v1.AgentService/GetUsableModels"
	GetDefaultModelPath  = "/agent.v1.AgentService/GetDefaultModelForCli"
	AvailableModelsPath  = "/aiserver.v1.AiService/AvailableModels"
	DashboardUsagePath   = "/aiserver.v1.DashboardService/GetCurrentPeriodUsage"
	ConnectVersionHeader = "Connect-Protocol-Version"
)

// Protocol bounds keep one hostile frame from exhausting the relay.
const (
	MaxFrameBytes  = 16 << 20
	MaxProtoBytes  = 8 << 20
	MaxBlobBytes   = 4 << 20
	MaxBlobs       = 256
	MaxHistoryTurn = 512
)

// Headers returns the headers every authenticated Cursor RPC carries.
func Headers(token, version, requestID string) map[string]string {
	if version == "" {
		version = ClientVersion
	}
	return map[string]string{
		"Content-Type":            "application/proto",
		"Authorization":           "Bearer " + token,
		"X-Cursor-Client-Type":    "cli",
		"X-Cursor-Client-Version": version,
		"X-Ghost-Mode":            "true",
		ConnectVersionHeader:      "1",
		"X-Request-Id":            requestID,
	}
}
