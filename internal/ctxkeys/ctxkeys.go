// Package ctxkeys defines shared, collision-resistant context.Context key
// types used to pass request-scoped values (such as the request ID)
// between middleware and handlers across packages.
package ctxkeys

// Key is a custom type for context keys to avoid collisions.
type Key string

const (
	// RequestID is the context key for request IDs.
	RequestID Key = "request_id"
)
