package ctxkeys

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestIDKey(t *testing.T) {
	// Verify the key constant value
	assert.Equal(t, Key("request_id"), RequestID)
}

func TestContextKeyAvoidCollision(t *testing.T) {
	ctx := context.Background()

	// Set value using our typed key
	ctx = context.WithValue(ctx, RequestID, "typed-value")

	// Set value using plain string key (should NOT collide)
	//nolint:staticcheck // intentionally using string key to test collision avoidance
	ctx = context.WithValue(ctx, "request_id", "string-value")

	// Typed key should return the typed value
	typedVal := ctx.Value(RequestID)
	require.NotNil(t, typedVal)
	assert.Equal(t, "typed-value", typedVal.(string))

	// String key should return the string value (different from typed key)
	//nolint:staticcheck // intentionally using string key to test collision avoidance
	stringVal := ctx.Value("request_id")
	require.NotNil(t, stringVal)
	assert.Equal(t, "string-value", stringVal.(string))
}

func TestKeyType(t *testing.T) {
	// Verify Key type is distinct from string
	var k Key = "test"
	assert.Equal(t, "test", string(k))
}
