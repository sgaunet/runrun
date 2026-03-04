# RunRun

**RunRun** is a web-based task execution platform written in Go that allows scheduling, executing, and monitoring shell command tasks through a modern web interface with real-time streaming updates.

## Features

- 🚀 **Task Execution** - Execute shell commands and multi-step tasks with configurable timeouts
- 📊 **Real-time Monitoring** - WebSocket-based live log streaming with automatic updates
- 🔐 **Secure Authentication** - JWT-based authentication with bcrypt password hashing
- 🛡️ **CSRF Protection** - Built-in CSRF token validation for state-changing operations
- ⏱️ **Rate Limiting** - Configurable rate limiting to prevent abuse
- 📝 **Task History** - Complete execution history with detailed logs and status tracking
- 🎨 **Modern UI** - Clean, responsive web interface built with Templ and Tailwind CSS
- 🔄 **Concurrent Execution** - Worker pool-based concurrent task processing
- 📁 **Log Management** - Automatic log file creation and management with tail support
- 🔍 **Health Checks** - Built-in health and readiness endpoints for monitoring

## Tech Stack

- **Backend**: Go 1.25+ with Chi router
- **Templating**: [Templ](https://templ.guide/) for type-safe HTML templates
- **Styling**: Tailwind CSS with custom configuration
- **Authentication**: JWT tokens with bcrypt password hashing
- **Real-time**: WebSocket for live log streaming
- **Testing**: Testify for comprehensive test coverage

## Installation

### Prerequisites

- Go 1.25 or higher
- Node.js (for Tailwind CSS build)
- Git

### From Source

```bash
# Clone the repository
git clone https://github.com/sgaunet/runrun.git
cd runrun

# Install development tools (templ CLI)
task install-tools

# Build everything (templates, CSS, and binary)
task build-all

# The binary will be in the current directory
./runrun version
```

### Using Go Install

```bash
go install github.com/sgaunet/runrun/cmd/runrun@latest
```

## Quick Start

### 1. Create Configuration File

Create a configuration file `config.yaml`:

```yaml
server:
  port: 8080
  log_level: "info"
  max_concurrent_tasks: 5
  session_timeout: 24h
  log_directory: "./logs"
  shutdown_timeout: 5m

auth:
  jwt_secret: "your-secret-key-at-least-32-characters-long"
  users:
    - username: "admin"
      password: "$2a$10$YourBcryptHashedPasswordHere"

tasks:
  - name: "hello-world"
    description: "Simple hello world task"
    tags: ["demo"]
    timeout: 1m
    steps:
      - name: "Echo Hello"
        command: "echo 'Hello, World!'"
```

### 2. Generate Password Hash

```bash
./runrun hash-password yourpassword
```

Copy the output hash to your config file.

### 3. Start the Server

```bash
./runrun server --config config.yaml --port 8080
```

### 4. Access the Web Interface

Open your browser and navigate to:
```
http://localhost:8080
```

Login with your configured username and password.

## Configuration

### Server Settings

| Option | Description | Default |
|--------|-------------|---------|
| `port` | HTTP server port | 8080 |
| `log_level` | Logging level (debug, info, warn, error) | info |
| `max_concurrent_tasks` | Maximum concurrent task executions | 5 |
| `session_timeout` | JWT session timeout duration | 24h |
| `log_directory` | Directory for task execution logs | ./logs |
| `shutdown_timeout` | Graceful shutdown timeout | 5m |

### Authentication

```yaml
auth:
  jwt_secret: "minimum-32-characters-secret-key"
  users:
    - username: "admin"
      password: "$2a$10$..."  # BCrypt hash
    - username: "user"
      password: "$2a$10$..."
```

**Important**: JWT secret must be at least 32 characters long for security.

### Task Configuration

```yaml
tasks:
  - name: "backup-database"
    description: "Backup PostgreSQL database"
    tags: ["database", "backup"]
    timeout: 30m
    working_directory: "/var/backups"
    environment:
      DB_HOST: "localhost"
      DB_NAME: "myapp"
    steps:
      - name: "Create backup directory"
        command: "mkdir -p /var/backups/$(date +%Y%m%d)"
      - name: "Run pg_dump"
        command: "pg_dump -h $DB_HOST $DB_NAME > backup.sql"
      - name: "Compress backup"
        command: "gzip backup.sql"
```

#### Task Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique task identifier |
| `description` | No | Human-readable description |
| `tags` | No | Array of tags for categorization |
| `timeout` | No | Maximum execution time (default: 5m) |
| `working_directory` | No | Working directory for command execution |
| `environment` | No | Environment variables (key-value pairs) |
| `steps` | Yes | Array of commands to execute sequentially |

## Development

### Building from Source

```bash
# Install development dependencies
task install-tools

# Generate template code from .templ files
task generate

# Build Tailwind CSS
task build-css

# Build the application
task build

# Run all tests
task test

# Run with hot reload
task watch
```

### Project Structure

```
runrun/
├── cmd/runrun/          # CLI application entry point
├── configs/             # Example configuration files
├── internal/
│   ├── auth/           # Authentication & authorization
│   ├── config/         # Configuration management
│   ├── csrf/           # CSRF protection
│   ├── executor/       # Task execution engine
│   ├── middleware/     # HTTP middleware
│   ├── ratelimit/      # Rate limiting
│   ├── server/         # HTTP server & handlers
│   ├── templates/      # Templ templates
│   └── websocket/      # WebSocket hub & client management
├── Taskfile.yml        # Task runner commands
└── tailwind.config.js  # Tailwind CSS configuration
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test ./... -cover

# Run specific package tests
go test ./internal/auth/...

# Run tests verbosely
go test -v ./...
```

## API Documentation

### REST Endpoints

#### Authentication

**POST** `/login`
```json
{
  "username": "admin",
  "password": "yourpassword"
}
```

Response:
```json
{
  "message": "Login successful"
}
```

Sets session cookie for authentication.

**POST** `/logout`

Clears session and invalidates token.

#### Task Execution

**POST** `/tasks/:name/execute`

Requires authentication. CSRF token required.

Response:
```json
{
  "execution_id": "abc12345-6789-...",
  "message": "Task queued for execution"
}
```

**GET** `/api/status`

Get status of all tasks and system statistics.

Response:
```json
{
  "tasks": [
    {
      "name": "hello-world",
      "description": "Simple hello world task",
      "tags": ["demo"],
      "status": "success",
      "last_run": "2025-01-15T10:30:00Z",
      "duration": 5.0
    }
  ],
  "stats": {
    "total": 10,
    "running": 1,
    "success": 7,
    "failed": 2,
    "queued": 0
  }
}
```

#### Logs

**GET** `/logs/:executionID`

View execution logs in browser.

**GET** `/logs/:executionID/download`

Download execution logs as file.

**GET** `/logs/:executionID/poll?lines=N`

Poll for log entries. Optional `lines` parameter returns last N lines.

#### Health Checks

**GET** `/health`

Overall health check.

**GET** `/health/ready`

Readiness probe for Kubernetes/container orchestration.

**GET** `/health/live`

Liveness probe.

### WebSocket Protocol

#### Connection

Connect to WebSocket endpoint with authentication:

```
ws://localhost:8080/logs/ws/{executionID}
```

Authentication via:
- Session cookie (from web login)
- Authorization header: `Bearer {jwt-token}`

#### Message Types

**Client → Server:**

Subscribe to execution:
```json
{
  "type": "subscribe",
  "execution_id": "abc12345-..."
}
```

Unsubscribe:
```json
{
  "type": "unsubscribe",
  "execution_id": "abc12345-..."
}
```

Ping:
```json
{
  "type": "pong"
}
```

**Server → Client:**

Log message:
```json
{
  "type": "log",
  "execution_id": "abc12345-...",
  "data": {
    "line": "Step completed successfully",
    "timestamp": "2025-01-15T10:30:05Z",
    "level": "info"
  },
  "timestamp": "2025-01-15T10:30:05Z"
}
```

Subscription confirmed:
```json
{
  "type": "subscribed",
  "execution_id": "abc12345-...",
  "timestamp": "2025-01-15T10:30:00Z"
}
```

Error:
```json
{
  "type": "error",
  "error": "Error message",
  "timestamp": "2025-01-15T10:30:00Z"
}
```

Ping (heartbeat):
```json
{
  "type": "ping",
  "timestamp": "2025-01-15T10:30:00Z"
}
```

## Security

### Authentication & Authorization

- All passwords are hashed using bcrypt (cost factor 10)
- JWT tokens for session management with configurable timeout
- Two-tier token validation: JWT signature + session store (enables revocation)
- Session cookies are HTTP-only and use SameSite=Strict

### CSRF Protection

- CSRF tokens required for all state-changing operations (POST, PUT, DELETE)
- Tokens can be provided via:
  - `X-CSRF-Token` header
  - `csrf_token` form field
- Token validation uses constant-time comparison

### Rate Limiting

- Configurable rate limiting on login endpoint (default: 5 attempts per 15 minutes)
- IP-based visitor tracking
- Automatic cleanup of old visitor entries

### WebSocket Security

- Manual authentication in WebSocket handler (outside middleware chain)
- Origin validation prevents Cross-Site WebSocket Hijacking (CSWSH)
- Supports both session cookie and Authorization Bearer token

### HTTP Security Headers

- Content-Security-Policy (CSP)
- X-Frame-Options: DENY
- X-Content-Type-Options: nosniff
- X-XSS-Protection
- Referrer-Policy
- Permissions-Policy
- Strict-Transport-Security (HSTS) for HTTPS

## Deployment

### Docker

Create a `Dockerfile`:

```dockerfile
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o runrun cmd/runrun/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/runrun .
COPY configs/production.yaml ./config.yaml

EXPOSE 8080
CMD ["./runrun", "server", "--config", "config.yaml"]
```

Build and run:

```bash
docker build -t runrun:latest .
docker run -p 8080:8080 -v $(pwd)/config.yaml:/root/config.yaml runrun:latest
```

### Kubernetes

Example deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: runrun
spec:
  replicas: 2
  selector:
    matchLabels:
      app: runrun
  template:
    metadata:
      labels:
        app: runrun
    spec:
      containers:
      - name: runrun
        image: runrun:latest
        ports:
        - containerPort: 8080
        env:
        - name: SERVER_PORT
          value: "8080"
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        volumeMounts:
        - name: config
          mountPath: /config
      volumes:
      - name: config
        configMap:
          name: runrun-config
```

### Systemd Service

Create `/etc/systemd/system/runrun.service`:

```ini
[Unit]
Description=RunRun Task Execution Platform
After=network.target

[Service]
Type=simple
User=runrun
Group=runrun
WorkingDirectory=/opt/runrun
ExecStart=/opt/runrun/runrun server --config /etc/runrun/config.yaml
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable runrun
sudo systemctl start runrun
```

## Troubleshooting

### Common Issues

#### "JWT secret must be at least 32 characters"

**Solution**: Generate a strong secret:
```bash
openssl rand -base64 32
```

Add to your config file.

#### WebSocket connection fails

**Causes**:
- Origin validation blocking connection
- Missing or invalid authentication token
- Execution ID doesn't exist

**Solution**:
- Check browser console for errors
- Verify authentication (login first)
- Ensure execution ID is correct

#### Task execution hangs

**Causes**:
- Task timeout not configured
- Command waiting for input
- Queue is full

**Solution**:
- Set appropriate timeout in task configuration
- Ensure commands don't require interactive input
- Increase `max_concurrent_tasks` in config

#### "Queue is full" error

**Solution**: Increase `max_concurrent_tasks` in server configuration:

```yaml
server:
  max_concurrent_tasks: 10  # Increase from default 5
```

#### Template changes not reflected

**Solution**: Regenerate template code:

```bash
task generate
task build
```

#### CSS changes not appearing

**Solution**: Rebuild Tailwind CSS:

```bash
task build-css
```

### Debug Mode

Enable debug logging:

```yaml
server:
  log_level: "debug"
```

## FAQ

**Q: Can I run tasks in parallel?**

A: Yes, RunRun uses a worker pool. The number of concurrent tasks is controlled by `max_concurrent_tasks` in the configuration.

**Q: How do I add new tasks?**

A: Add them to your `config.yaml` file in the `tasks` section and restart the server.

**Q: Can I trigger tasks via API?**

A: Yes, use the `POST /tasks/:name/execute` endpoint with proper authentication.

**Q: How long are logs kept?**

A: Logs are stored permanently in the `log_directory`. Implement your own rotation/cleanup if needed.

**Q: Can I use environment variables in commands?**

A: Yes, define them in the task's `environment` section or use system environment variables.

**Q: Is it safe to expose RunRun to the internet?**

A: RunRun has security features (auth, CSRF, rate limiting), but additional hardening is recommended:
- Use HTTPS (reverse proxy with SSL/TLS)
- Implement network-level access controls
- Regular security updates
- Strong passwords and secrets

**Q: How do I backup my configuration?**

A: Simply backup your `config.yaml` file. All task definitions and user credentials are stored there.

**Q: Can I integrate with CI/CD pipelines?**

A: Yes, use the REST API to trigger tasks. Example with curl:

```bash
# Login and get session cookie
curl -c cookies.txt -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"yourpass"}'

# Execute task
curl -b cookies.txt -X POST http://localhost:8080/tasks/my-task/execute
```

## Contributing

Contributions are welcome! Please follow these guidelines:

1. **Fork the repository**
2. **Create a feature branch**: `git checkout -b feature/amazing-feature`
3. **Write tests** for new functionality
4. **Ensure tests pass**: `task test`
5. **Format code**: `go fmt ./...`
6. **Commit changes**: `git commit -m 'Add amazing feature'`
7. **Push to branch**: `git push origin feature/amazing-feature`
8. **Open a Pull Request**

### Development Setup

```bash
# Clone your fork
git clone https://github.com/yourusername/runrun.git
cd runrun

# Install tools
task install-tools

# Run tests with coverage
go test ./... -cover

# Build and test locally
task build
./runrun server --config configs/example.yaml
```

### Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Write tests for new features
- Update documentation for API changes

## License

[Include your license information here]

## Support

- **Issues**: [GitHub Issues](https://github.com/sgaunet/runrun/issues)
- **Discussions**: [GitHub Discussions](https://github.com/sgaunet/runrun/discussions)

## Acknowledgments

- Built with [Chi](https://github.com/go-chi/chi) router
- Templates with [Templ](https://templ.guide/)
- Styled with [Tailwind CSS](https://tailwindcss.com/)
- WebSocket support via [Gorilla WebSocket](https://github.com/gorilla/websocket)
