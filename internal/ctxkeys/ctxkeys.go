package ctxkeys

// Key is a custom type for context keys to avoid collisions.
type Key string

const (
	// RequestID is the context key for request IDs.
	RequestID Key = "request_id"
)
