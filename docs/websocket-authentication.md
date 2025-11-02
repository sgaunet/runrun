# WebSocket Authentication

## Overview

The runrun application implements secure WebSocket connections for real-time log streaming. This document describes the authentication flow, security considerations, and implementation details.

## Authentication Flow

### Why Manual Authentication?

WebSocket endpoints in runrun are configured **outside the standard middleware chain** for a critical reason:

```go
// In routes.go - WebSocket routes are NOT in the compressed group
r.Get("/logs/ws/{executionID}", s.wsLogsHandler)

// Other routes use middleware chain
r.Group(func(r chi.Router) {
    r.Use(middleware.Compress(5))
    // ... auth middleware, etc.
})
```

**Reason**: The WebSocket upgrade process requires the `http.Hijacker` interface to take control of the TCP connection. Middleware that wraps the `ResponseWriter` (like compression middleware) can break this interface, preventing the WebSocket upgrade from succeeding.

Therefore, authentication must be performed **manually** within the `wsLogsHandler` before upgrading the connection.

### Authentication Methods

The WebSocket endpoint supports two authentication methods, checked in order:

1. **Session Cookie** (checked first)
   ```http
   Cookie: session=<jwt-token>
   ```

2. **Authorization Header** (fallback)
   ```http
   Authorization: Bearer <jwt-token>
   ```

### Authentication Sequence

```
Client Request
    ↓
1. Extract token from cookie OR Authorization header
    ↓
2. Validate token is present
    ↓ (if missing)
    Return 401 Unauthorized
    ↓ (if present)
3. Validate JWT signature and expiration
    ↓ (if invalid)
    Return 401 Unauthorized
    ↓ (if valid)
4. Check session exists in server session store
    ↓ (if not found)
    Return 401 Unauthorized
    ↓ (if found)
5. Verify execution exists
    ↓ (if not found)
    Return 404 Not Found
    ↓ (if found)
6. Upgrade to WebSocket connection
    ↓
7. Stream logs to authenticated client
```

## Security Considerations

### 1. Origin Validation (CORS)

The WebSocket upgrader implements strict origin validation:

```go
CheckOrigin: func(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    if origin == "" {
        return true // Allow requests with no origin (non-browser clients)
    }

    originURL, err := url.Parse(origin)
    if err != nil {
        return false
    }

    // Only allow same-origin requests
    if originURL.Host != r.Host {
        log.Printf("WebSocket origin rejected: %s (expected: %s)", originURL.Host, r.Host)
        return false
    }

    return true
}
```

**Security Notes**:
- Browser-based WebSocket connections include an `Origin` header
- Only connections from the same host are allowed
- Non-browser clients (CLI tools, scripts) without Origin header are permitted
- Different schemes (http vs https) on same host are allowed

### 2. Session-Based Token Validation

Authentication uses a **two-tier validation**:

1. **JWT Token Validation**: Verifies signature and expiration
2. **Session Store Validation**: Ensures token exists in active sessions

This prevents use of:
- Revoked tokens (removed from session store)
- Expired sessions (even if JWT hasn't expired yet)
- Tokens from previous server instances

### 3. No Credentials in URL

The authentication token is **never** passed in the WebSocket URL:

❌ **Bad** (token exposed in logs, browser history):
```javascript
new WebSocket(`wss://example.com/logs/ws/${executionID}?token=${token}`)
```

✅ **Good** (token in secure headers):
```javascript
// Not directly supported by browser WebSocket API - use subprotocols or cookies
// For browser: rely on automatic cookie inclusion
const ws = new WebSocket(`wss://example.com/logs/ws/${executionID}`)

// For programmatic access: use headers via custom HTTP library, then upgrade
```

### 4. Connection Lifecycle Management

The implementation prevents reconnection loops:

```go
// If execution finished, keep connection open for 30s to prevent reconnect loop
if execution.FinishedAt != nil {
    // Send final status
    conn.WriteJSON(msg)

    // Wait for client to close or timeout
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    select {
    case <-ticker.C:
        return
    case <-r.Context().Done():
        return
    }
}
```

## Client-Side Implementation Examples

### Browser (JavaScript)

```javascript
// Browser automatically includes cookies in WebSocket requests
async function connectToLogs(executionID) {
    // Ensure user is authenticated (session cookie is set)
    const ws = new WebSocket(`wss://${window.location.host}/logs/ws/${executionID}`);

    ws.onopen = () => {
        console.log('Connected to log stream');
    };

    ws.onmessage = (event) => {
        const message = JSON.parse(event.data);
        if (message.type === 'log') {
            console.log(message.data.line);
        }
    };

    ws.onerror = (error) => {
        console.error('WebSocket error:', error);
    };

    ws.onclose = (event) => {
        if (event.code === 1006) {
            console.error('Connection closed abnormally - possible auth failure');
        }
    };

    return ws;
}
```

### CLI/Programmatic (Go)

```go
import (
    "github.com/gorilla/websocket"
    "net/http"
)

func connectToLogs(executionID, token string) (*websocket.Conn, error) {
    headers := http.Header{}
    headers.Add("Authorization", "Bearer " + token)

    dialer := websocket.Dialer{}
    conn, resp, err := dialer.Dial(
        fmt.Sprintf("ws://localhost:8080/logs/ws/%s", executionID),
        headers,
    )

    if err != nil {
        if resp != nil && resp.StatusCode == 401 {
            return nil, fmt.Errorf("authentication failed")
        }
        return nil, err
    }

    return conn, nil
}
```

### CLI/Programmatic (Python)

```python
import websockets
import asyncio

async def connect_to_logs(execution_id: str, token: str):
    headers = {
        "Authorization": f"Bearer {token}"
    }

    uri = f"ws://localhost:8080/logs/ws/{execution_id}"

    async with websockets.connect(uri, extra_headers=headers) as websocket:
        async for message in websocket:
            data = json.loads(message)
            if data["type"] == "log":
                print(data["data"]["line"], end="")
```

### curl + websocat

```bash
# First, login and get session token
TOKEN=$(curl -s -X POST http://localhost:8080/login \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"secret"}' \
    | jq -r '.token')

# Connect to WebSocket with token
websocat -H "Authorization: Bearer $TOKEN" \
    ws://localhost:8080/logs/ws/execution-123
```

## Configuration

### JWT Secret

The JWT secret must be configured in the application config:

```yaml
auth:
  jwt_secret: "your-secret-key-at-least-32-characters-long"
  users:
    - username: admin
      password: $2a$10$...  # bcrypt hash
```

**Security Requirements**:
- Minimum 32 characters
- Use cryptographically random values
- Rotate periodically
- Store securely (environment variables, secrets management)

### Session Timeout

Configure session timeout in server config:

```yaml
server:
  session_timeout: 24h  # Sessions expire after 24 hours
```

Sessions are validated on every WebSocket connection attempt.

## Testing

Comprehensive tests are available in `internal/server/handlers_websocket_test.go`:

```bash
# Run WebSocket authentication tests
go test -v ./internal/server/... -run TestWebSocketAuth

# Run origin validation tests
go test -v ./internal/server/... -run TestWebSocketCheckOrigin
```

### Test Coverage

- ✅ Valid session cookie authentication
- ✅ Valid Authorization header authentication
- ✅ Missing authentication (401)
- ✅ Invalid tokens (401)
- ✅ Malformed Authorization headers (401)
- ✅ Expired tokens (401)
- ✅ Non-existent execution (404)
- ✅ Multiple auth methods (cookie precedence)
- ✅ Origin validation (same-origin policy)

## Common Issues

### 1. WebSocket Connection Immediately Closes

**Symptom**: Connection opens then immediately closes

**Causes**:
- Invalid or expired authentication token
- Session not found in server session store
- Execution ID doesn't exist

**Solution**: Check browser console or client logs for 401/404 errors

### 2. CORS Origin Errors

**Symptom**: `WebSocket origin rejected` in server logs

**Cause**: Client origin doesn't match server host

**Solution**:
- Ensure client connects from same origin as server
- For development with different ports, update CheckOrigin logic
- For production, use proper domain configuration

### 3. Authentication Works for HTTP but Not WebSocket

**Symptom**: Regular API calls work, WebSocket fails with 401

**Cause**: Session cookie not included in WebSocket request

**Solution**:
- Verify cookies are enabled in browser
- Check cookie domain and path settings
- Ensure HTTPS is used in production (secure cookies)

### 4. Connection Timeout Before Logs Appear

**Symptom**: WebSocket closes before any log output

**Cause**: Log file hasn't been created yet (task not started)

**Solution**: Implementation includes waiting logic:
- Waits up to 30 seconds for log file creation
- Sends "Waiting for log output..." message
- Polls every 500ms for log file

## Architecture Decisions

### Why Not Use Middleware?

**Decision**: WebSocket endpoint is outside middleware chain

**Rationale**:
1. WebSocket upgrade requires `http.Hijacker` interface
2. Middleware can wrap `ResponseWriter` and break Hijacker
3. Manual authentication provides same security guarantees
4. More explicit and testable

### Why Session Store in Addition to JWT?

**Decision**: Validate both JWT and session store

**Rationale**:
1. Enables token revocation (logout)
2. Server restart invalidates old tokens
3. Session timeout separate from JWT expiration
4. Better control over active sessions

### Why Allow Requests Without Origin Header?

**Decision**: Permit WebSocket connections without Origin header

**Rationale**:
1. CLI tools and scripts don't send Origin
2. Authentication still required (token validation)
3. Origin validation still protects browser-based attacks
4. Enables automation and monitoring tools

## References

- [RFC 6455 - The WebSocket Protocol](https://tools.ietf.org/html/rfc6455)
- [OWASP WebSocket Security](https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html)
- [gorilla/websocket Documentation](https://pkg.go.dev/github.com/gorilla/websocket)

## Related Code

- `internal/server/handlers.go` - `wsLogsHandler()` implementation
- `internal/server/routes.go` - Route configuration
- `internal/auth/service.go` - Authentication service
- `internal/middleware/middleware.go` - Middleware implementations
- `internal/server/handlers_websocket_test.go` - Test suite
