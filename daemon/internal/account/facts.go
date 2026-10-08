package account

// ContextFact is one account's stored context-window override for one model.
type ContextFact struct {
	CredentialID  string
	ProviderID    string
	ModelID       string
	ContextWindow *int64
	UpdatedAtMs   int64
}
