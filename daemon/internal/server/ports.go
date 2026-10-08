// Server ports: the narrow sources the HTTP layer reads instead of adapters.
package server

// SchemaVersionSource reports the state schema version the status page
// shows. The SQLite database satisfies it structurally, so this package
// never imports the persistence adapter.
type SchemaVersionSource interface {
	SchemaVersion() (int, error)
}

// SecretModeSource names the secret backend the status page shows. The
// platform wraps the concrete store, so this package never imports the
// secrets adapter.
type SecretModeSource interface {
	Mode() string
}
