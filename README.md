# ebuse - Remote Event Storage Server for ebu

`ebuse` is a remote event storage server that provides persistent storage for the [ebu](https://github.com/jilio/ebu) event bus library. It uses SQLite for storage and provides an HTTP API with API key authentication.

## Features

### Core Features

- **Remote Event Storage**: Store events from `ebu` event bus remotely
- **SQLite Persistence**: Production-tuned SQLite with WAL mode, prepared statements, and optimized indexes
- **Multi-Tenant Support**: Complete data isolation with per-tenant databases
- **API Key Authentication**: Secure access using environment-based or YAML-configured API keys
- **Event Replay**: Load historical events with position tracking
- **Subscription Tracking**: Maintain subscription positions for resumable event processing

### Production Features

- **High Performance**: 20,000+ events/sec single writes, 50,000+ events/sec batch writes
- **Batch Operations**: Insert up to 1000 events in a single transaction
- **Streaming API**: Stream millions of events without loading all into memory
- **Rate Limiting**: Configurable per-IP rate limiting (default: 100 req/s)
- **Gzip Compression**: Automatic compression for large responses
- **Connection Pooling**: Optimized connection management (25 max, 10 idle)
- **Health Checks**: `/health` endpoint for load balancers
- **Metrics**: `/metrics` endpoint for monitoring (shows tenant name in multi-tenant mode)
- **Graceful Shutdown**: Proper signal handling and connection draining

## Installation

```bash
go get github.com/jilio/ebuse
```

## Server Setup

### Single-Tenant Mode (Simple)

For a single database with one API key:

```bash
# Set required environment variables
export API_KEY="your-secret-api-key"
export PORT="8080"              # Optional, defaults to 8080
export DB_PATH="events.db"      # Optional, defaults to events.db

# Run the server
go run ./cmd/ebuse
```

### Multi-Tenant Mode (Recommended for SaaS)

For multiple isolated databases with separate API keys:

1. Create a `tenants.yaml` configuration file:

```yaml
data_dir: "data"  # Directory for tenant databases

tenants:
  - name: "customer-a"
    api_key: "customer-a-secret-key"

  - name: "customer-b"
    api_key: "customer-b-secret-key"

  - name: "customer-c"
    api_key: "customer-c-secret-key"
```

2. Run the server with the config file:

```bash
export PORT="8080"  # Optional configuration
./ebuse -config tenants.yaml
```

Each tenant gets:
- Isolated database: `data/{tenant-name}.db`
- Independent event positions
- Complete data separation
- Own metrics and statistics

### Docker (Optional)

```dockerfile
FROM golang:1.24-alpine

WORKDIR /app
COPY . .
RUN go build -o ebuse ./cmd/ebuse

ENV API_KEY=""
ENV PORT="8080"
ENV DB_PATH="/data/events.db"

EXPOSE 8080
CMD ["./ebuse"]
```

## Client Usage

### Basic Usage with ebu

```go
package main

import (
    "github.com/jilio/ebu"
    "github.com/jilio/ebuse/pkg/client"
)

func main() {
    // Create remote event store client
    remoteStore := client.New(
        "http://localhost:8080",
        "your-secret-api-key",
    )

    // Create event bus with remote persistence
    bus := eventbus.New(
        eventbus.WithStore(remoteStore),
    )

    // Use the event bus normally
    type UserCreated struct {
        ID   string
        Name string
    }

    eventbus.Subscribe(bus, func(e UserCreated) {
        fmt.Printf("User created: %s\n", e.Name)
    })

    eventbus.Publish(bus, UserCreated{
        ID:   "123",
        Name: "Alice",
    })
}
```

### Direct API Usage

#### Save Event

```bash
curl -X POST http://localhost:8080/events \
  -H "X-API-Key: your-secret-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "UserCreated",
    "data": {"id":"123","name":"Alice"},
    "timestamp": "2024-01-01T00:00:00Z"
  }'
```

#### Load Events

```bash
# Load all events from position 1
curl -X GET "http://localhost:8080/events?from=1" \
  -H "X-API-Key: your-secret-api-key"

# Load events in range
curl -X GET "http://localhost:8080/events?from=1&to=10" \
  -H "X-API-Key: your-secret-api-key"
```

#### Get Current Position

```bash
curl -X GET http://localhost:8080/position \
  -H "X-API-Key: your-secret-api-key"
```

#### Save Subscription Position

```bash
curl -X POST http://localhost:8080/subscriptions/my-subscription/position \
  -H "X-API-Key: your-secret-api-key" \
  -H "Content-Type: application/json" \
  -d '{"position": 42}'
```

#### Load Subscription Position

```bash
curl -X GET http://localhost:8080/subscriptions/my-subscription/position \
  -H "X-API-Key: your-secret-api-key"
```

## API Reference

### Authentication

All requests require authentication via API key. Provide the key using one of these headers:

- `X-API-Key: your-key`
- `Authorization: Bearer your-key`

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | /events | Save a new event |
| POST | /events/batch | Save up to 1000 events (bulk insert) |
| GET | /events?from={position}&to={position} | Load events (max 10k, to is optional) |
| GET | /events/stream?from={position}&batch_size={size} | Stream events (for large replays) |
| GET | /position | Get current event position |
| POST | /subscriptions/{id}/position | Save subscription position |
| GET | /subscriptions/{id}/position | Load subscription position |
| GET | /health | Health check (for load balancers, no auth) |
| GET | /metrics | Metrics with tenant info (requires auth) |
| GET | /tenants | List all tenants (multi-tenant mode only, requires auth) |

## Examples

### Direct API Usage

```bash
# Start the server first
./run.sh

# In another terminal, run the direct API example
go run ./example/direct
```

### Integration with ebu

```bash
# See the integration example (requires ebu library)
go run ./example/integration
```

### Multi-Tenant Example

```bash
# 1. Start server in multi-tenant mode
./ebuse -config tenants.yaml

# 2. Publish event to tenant "alice"
curl -X POST http://localhost:8080/events \
  -H "X-API-Key: alice-key-123" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "OrderPlaced",
    "data": {"order_id": "A-001", "amount": 99.99},
    "timestamp": "2024-01-01T00:00:00Z"
  }'

# 3. Publish event to tenant "bob"
curl -X POST http://localhost:8080/events \
  -H "X-API-Key: bob-key-456" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "OrderPlaced",
    "data": {"order_id": "B-001", "amount": 149.50},
    "timestamp": "2024-01-01T00:00:00Z"
  }'

# 4. Check alice's metrics (isolated from bob)
curl -H "X-API-Key: alice-key-123" http://localhost:8080/metrics

# 5. Check bob's metrics (isolated from alice)
curl -H "X-API-Key: bob-key-456" http://localhost:8080/metrics

# 6. List all tenants (with any valid API key)
curl -H "X-API-Key: alice-key-123" http://localhost:8080/tenants
```

## Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/store
go test ./pkg/server
```

## Architecture

```
ebuse/
├── cmd/ebuse/              # Server entry point
├── internal/store/        # SQLite storage implementation
├── pkg/
│   ├── client/            # HTTP client (implements ebu's EventStore)
│   └── server/            # HTTP server with auth
├── example/
│   ├── direct/            # Direct API usage example
│   └── integration/       # ebu integration example
└── README.md
```

### Storage Schema

**events table:**

- `position` (INTEGER PRIMARY KEY) - Auto-incrementing event position
- `type` (TEXT) - Event type name
- `data` (BLOB) - JSON-encoded event data
- `timestamp` (DATETIME) - Event timestamp

**subscriptions table:**

- `subscription_id` (TEXT PRIMARY KEY) - Unique subscription identifier
- `position` (INTEGER) - Last processed position

## Configuration

### Environment Variables (Both Modes)

| Variable | Default | Description |
|----------|---------|-------------|
| PORT | 8080 | HTTP server port |
| RATE_LIMIT | 100 | Requests per second per IP |
| RATE_BURST | 200 | Burst size for rate limiter |
| ENABLE_GZIP | true | Enable gzip compression |
| READ_TIMEOUT | 30s | HTTP read timeout |
| WRITE_TIMEOUT | 60s | HTTP write timeout |
| IDLE_TIMEOUT | 120s | HTTP idle timeout |
| SHUTDOWN_TIMEOUT | 30s | Graceful shutdown timeout |

### Single-Tenant Mode Only

| Variable | Default | Description |
|----------|---------|-------------|
| **API_KEY** | *(required)* | API key for authentication |
| DB_PATH | events.db | SQLite database file path |

### Multi-Tenant Mode Only

Create a `tenants.yaml` file:

```yaml
# Optional: Directory for tenant databases (default: "data")
data_dir: "data"

# Required: List of tenants
tenants:
  - name: "tenant-name"      # Database will be: data/tenant-name.db
    api_key: "unique-key"    # API key for this tenant
```

Run with: `./ebuse -config tenants.yaml`

### Choosing a Mode

**Use Single-Tenant Mode when:**
- You have a single application/customer
- Simplicity is preferred
- Quick setup needed

**Use Multi-Tenant Mode when:**
- Building a SaaS application
- Need data isolation between customers
- Managing multiple environments (dev/staging/prod)
- Want centralized tenant management

For production deployment, tuning, and performance benchmarks, see [PRODUCTION.md](PRODUCTION.md).

## License

Same as [ebu](https://github.com/jilio/ebu)
