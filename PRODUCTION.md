# ebuse Production Deployment Guide

## Performance Characteristics

### Benchmarks (Apple M1 Pro)

```
BenchmarkSave                    26,247 ops    46.4 µs/op      472 B/op    15 allocs/op
BenchmarkSaveBatch/size-10        5,703 ops   250.9 µs/op    5,370 B/op   161 allocs/op
BenchmarkSaveBatch/size-100         830 ops     2.1 ms/op   47,810 B/op  1,511 allocs/op
BenchmarkSaveBatch/size-1000        100 ops    19.5 ms/op  472,655 B/op 15,011 allocs/op
BenchmarkLoad (10k events)           75 ops    13.7 ms/op    4.1 MB/op
BenchmarkLoadStream (10k)            86 ops    13.3 ms/op    3.9 MB/op
BenchmarkConcurrentSave          23,475 ops    51.1 µs/op      472 B/op    15 allocs/op
BenchmarkGetPosition            172,438 ops     6.7 µs/op      432 B/op    14 allocs/op
```

### Key Performance Metrics

- **Single Event Save**: ~46 µs (21,000 events/second)
- **Batch Save (1000 events)**: ~19.5 ms (51,000 events/second)
- **Concurrent Save**: ~51 µs per operation
- **Position Query**: ~6.7 µs (149,000 queries/second)
- **Load 10k Events**: ~13.7 ms
- **Stream 10k Events**: ~13.3 ms (slightly better memory usage)

## Production Configuration

### Deployment Modes

**Single-Tenant Mode:**
- One database, one API key
- Simpler setup
- Use for: single application, quick deployments

**Multi-Tenant Mode:**
- Multiple isolated databases
- Per-tenant API keys
- Use for: SaaS, multi-customer, multi-environment

### Environment Variables (Both Modes)

| Variable | Default | Description |
|----------|---------|-------------|
| **PORT** | 8080 | HTTP server port |
| **RATE_LIMIT** | 100 | Requests per second per IP |
| **RATE_BURST** | 200 | Burst size for rate limiter |
| **ENABLE_GZIP** | true | Enable gzip compression for large responses |
| **READ_TIMEOUT** | 30s | HTTP read timeout |
| **WRITE_TIMEOUT** | 60s | HTTP write timeout |
| **IDLE_TIMEOUT** | 120s | HTTP idle connection timeout |
| **SHUTDOWN_TIMEOUT** | 30s | Graceful shutdown timeout |

### Single-Tenant Only

| Variable | Default | Description |
|----------|---------|-------------|
| **API_KEY** | *required* | Authentication key for all API requests |
| **DB_PATH** | events.db | SQLite database file path |

### Multi-Tenant Configuration

Create `tenants.yaml`:
```yaml
data_dir: "/data/tenants"  # Production path
tenants:
  - name: "prod-customer-a"
    api_key: "prod-key-a-very-long-random-string"
  - name: "prod-customer-b"
    api_key: "prod-key-b-very-long-random-string"
```

Start with: `./ebuse -config /etc/ebuse/tenants.yaml`

### Example Production Configuration

```bash
# Required
export API_KEY="your-secure-random-key-here"

# Server tuning
export PORT="8080"
export READ_TIMEOUT="30s"
export WRITE_TIMEOUT="60s"
export IDLE_TIMEOUT="120s"

# Rate limiting (per IP)
export RATE_LIMIT="100"   # 100 req/s
export RATE_BURST="200"   # Allow bursts up to 200

# Database
export DB_PATH="/data/events.db"

# Features
export ENABLE_GZIP="true"
```

## Database Optimizations

### SQLite Configuration (Automatic)

The server automatically applies production-ready SQLite settings:

```sql
PRAGMA journal_mode=WAL              -- Write-Ahead Logging for concurrency
PRAGMA synchronous=NORMAL            -- Balance safety/performance
PRAGMA cache_size=-64000             -- 64MB cache
PRAGMA busy_timeout=5000             -- 5s busy timeout
PRAGMA wal_autocheckpoint=1000       -- Checkpoint every 1000 pages
PRAGMA temp_store=MEMORY             -- Keep temp tables in memory
PRAGMA mmap_size=268435456           -- 256MB memory-mapped I/O
```

### Connection Pool Settings

```go
MaxOpenConns: 25                     -- Maximum open connections
MaxIdleConns: 10                     -- Maximum idle connections
ConnMaxLifetime: 5 minutes           -- Connection lifetime
ConnMaxIdleTime: 1 minute            -- Idle connection timeout
```

### Indexes

Optimized indexes for common query patterns:

- **Primary**: `position INTEGER PRIMARY KEY AUTOINCREMENT`
- **Type + Position**: Composite index for type-filtered queries
- **Timestamp**: Time-range queries
- **Covering Index**: Position + Type for replay without table lookups

## API Endpoints

### Production Endpoints

| Method | Path | Description | Use Case |
|--------|------|-------------|----------|
| POST | /events | Save single event | Real-time event ingestion |
| POST | /events/batch | Save up to 1000 events | Bulk ingestion |
| GET | /events?from=X&to=Y | Load events (max 10k) | Small replays |
| GET | /events/stream?from=X | Stream events | Large replays (millions) |
| GET | /position | Get current position | Status checks |
| POST/GET | /subscriptions/:id/position | Track subscription | Resumable consumers |
| GET | /health | Health check | Load balancers |
| GET | /metrics | Basic metrics | Monitoring |

### Choosing the Right Endpoint

**For Event Ingestion:**

- Single events < 100/s → Use `/events` (POST)
- Bursts or > 100/s → Use `/events/batch` (POST up to 1000 events)

**For Event Replay:**

- < 10,000 events → Use `/events?from=X&to=Y`
- > 10,000 events → Use `/events/stream?from=X&batch_size=1000`

**Streaming Example:**

```bash
curl -H "X-API-Key: secret" \
  "http://localhost:8080/events/stream?from=1&batch_size=1000"
```

This returns chunked JSON array, streaming events in batches without loading all into memory.

## Scaling Recommendations

### Small-Medium Load (< 10M events, < 1000 req/s)

- **Single Server**: Default configuration handles this easily
- **Storage**: Standard SSD
- **Memory**: 1-2 GB
- **CPU**: 2 cores

### Medium-High Load (10M-100M events, 1k-10k req/s)

- **Multiple Readers**: Run multiple read-only replicas
- **Storage**: NVMe SSD (WAL mode benefits from fast storage)
- **Memory**: 4-8 GB
- **CPU**: 4-8 cores
- **Rate Limits**: Increase to 1000 req/s per IP

### High Load (> 100M events, > 10k req/s)

- **Horizontal Sharding**: Shard events by type or time range
- **Read Replicas**: Use SQLite replication (Litestream)
- **Storage**: High-performance NVMe
- **Memory**: 16+ GB
- **CPU**: 8+ cores
- **Consider**: PostgreSQL migration for multi-writer scenarios

## Monitoring

### Health Check

```bash
curl http://localhost:8080/health
```

Returns:

```json
{"status": "healthy"}
```

Use this for load balancer health checks.

### Metrics

```bash
curl -H "X-API-Key: secret" http://localhost:8080/metrics
```

Returns:

```json
{
  "total_events": 1234567,
  "timestamp": 1704067200
}
```

### Recommended Monitoring

**Application Metrics:**

- Request rate (by endpoint)
- Error rate
- Response times (p50, p95, p99)
- Rate limit hits

**Database Metrics:**

- Event count growth rate
- Database file size
- WAL file size
- Checkpoint frequency

**System Metrics:**

- CPU usage
- Memory usage
- Disk I/O
- Disk space

## Backup and Disaster Recovery

### Backup Strategies

**Option 1: SQLite Backup API (Built-in)**

```bash
sqlite3 events.db ".backup events-backup.db"
```

**Option 2: File Copy (WAL-aware)**

```bash
sqlite3 events.db "PRAGMA wal_checkpoint(TRUNCATE);"
cp events.db events-backup.db
```

**Option 3: Litestream (Continuous Replication)**

```yaml
# litestream.yml
dbs:
  - path: /data/events.db
    replicas:
      - url: s3://mybucket/events
```

### Event Sourcing

Since all events are immutable and position-indexed, you can:

- Replay from any position
- Create point-in-time snapshots
- Rebuild state from events

## Security Best Practices

1. **API Key**:
   - Use strong, randomly generated keys (32+ characters)
   - Rotate keys periodically
   - Store in secrets management (not in code)

2. **Network**:
   - Run behind reverse proxy (nginx, caddy)
   - Use TLS (HTTPS)
   - Firewall rules to limit access

3. **Rate Limiting**:
   - Adjust based on expected load
   - Monitor rate limit hits
   - Consider per-API-key limits

4. **Database**:
   - Restrict file permissions (600)
   - Regular backups
   - Monitor disk space

## Troubleshooting

### High Memory Usage

**Cause**: Loading too many events at once
**Solution**: Use `/events/stream` instead of `/events`

### Slow Queries

**Cause**: Database not optimized or too many concurrent writes
**Solution**:

- Check indexes: `sqlite3 events.db ".schema"`
- Run `ANALYZE` manually
- Increase cache size

### Rate Limiting Issues

**Cause**: Legitimate traffic exceeding limits
**Solution**: Increase `RATE_LIMIT` and `RATE_BURST`

### Database Locked

**Cause**: WAL checkpoint or concurrent writers
**Solution**: Increase `busy_timeout` or reduce write concurrency

## Load Testing

### Example with wrk

```bash
# Single event writes
wrk -t4 -c100 -d30s \
  -H "X-API-Key: secret" \
  -H "Content-Type: application/json" \
  -s post-event.lua \
  http://localhost:8080/events

# Batch writes
wrk -t4 -c100 -d30s \
  -H "X-API-Key: secret" \
  -H "Content-Type: application/json" \
  -s post-batch.lua \
  http://localhost:8080/events/batch
```

### Expected Performance

On decent hardware (4-core, SSD):

- **Single writes**: 5,000-15,000 req/s
- **Batch writes (100 events)**: 500-1,500 req/s (50k-150k events/s)
- **Reads**: 10,000-20,000 req/s

## Production Checklist

- [ ] Set strong `API_KEY`
- [ ] Configure appropriate rate limits
- [ ] Set up monitoring (health checks, metrics)
- [ ] Configure backup strategy
- [ ] Set up log aggregation
- [ ] Configure reverse proxy with TLS
- [ ] Set resource limits (memory, disk)
- [ ] Test disaster recovery procedure
- [ ] Load test before production
- [ ] Document runbooks for common issues
