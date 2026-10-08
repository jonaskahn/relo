package inference

// The origins a usage event records. The value tells an operator which side
// of the console started a request: the data plane a client points at, or a
// test the operator ran from the Integrations page.
const (
	// OriginExternal names traffic a client sent through a data plane port.
	OriginExternal = "external"
	// OriginInternal names a chat the console's tester sent so an operator
	// can see a connection work without a client.
	OriginInternal = "internal"
)

// OriginKnown reports whether an origin identifier is one Relo stores.
func OriginKnown(origin string) bool {
	return origin == OriginExternal || origin == OriginInternal
}
