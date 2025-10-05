# Multi-Tenant Guide for ebus

## Overview

ebus supports true multi-tenancy with **complete data isolation** between tenants. Each tenant gets:
- Dedicated SQLite database file
- Independent API key
- Isolated event streams and positions
- Separate metrics and statistics

## Quick Start

### 1. Create Configuration File

Create `tenants.yaml`:

```yaml
# Directory for all tenant databases (optional, default: "data")
data_dir: "data"

# Define your tenants
tenants:
  - name: "alice"
    api_key: "alice-secret-key-123"

  - name: "bob"
    api_key: "bob-secret-key-456"

  - name: "charlie"
    api_key: "charlie-secret-key-789"
```

### 2. Start Server

```bash
./ebus -config tenants.yaml
```

Output:
```
2025/10/05 13:41:38 === ebus Multi-Tenant Server ===
2025/10/05 13:41:38 Config file: tenants.yaml
2025/10/05 13:41:38 Initialized 3 tenants: [alice bob charlie]
2025/10/05 13:41:38 Data directory: data
2025/10/05 13:41:38 Server listening on :8080
```

### 3. Use Different Tenants

```bash
# Publish to Alice's database
curl -X POST http://localhost:8080/events \
  -H "X-API-Key: alice-secret-key-123" \
  -H "Content-Type: application/json" \
  -d '{"type":"OrderPlaced","data":{"order":"A-001"},"timestamp":"2024-01-01T00:00:00Z"}'

# Publish to Bob's database
curl -X POST http://localhost:8080/events \
  -H "X-API-Key: bob-secret-key-456" \
  -H "Content-Type: application/json" \
  -d '{"type":"OrderPlaced","data":{"order":"B-001"},"timestamp":"2024-01-01T00:00:00Z"}'

# Check Alice's metrics (shows only Alice's data)
curl -H "X-API-Key: alice-secret-key-123" http://localhost:8080/metrics
# {"tenant":"alice","total_events":1,"timestamp":1759632128}

# Check Bob's metrics (shows only Bob's data)
curl -H "X-API-Key: bob-secret-key-456" http://localhost:8080/metrics
# {"tenant":"bob","total_events":1,"timestamp":1759632128}
```

## Database Files

Each tenant gets their own SQLite database:

```
data/
├── alice.db
├── alice.db-shm
├── alice.db-wal
├── bob.db
├── bob.db-shm
├── bob.db-wal
├── charlie.db
├── charlie.db-shm
└── charlie.db-wal
```

## Complete Data Isolation

**Event Positions are Independent:**
```bash
# Alice's first event gets position 1
curl -X POST http://localhost:8080/events \
  -H "X-API-Key: alice-secret-key-123" \
  -d '...'
# Response: {"position":1,...}

# Bob's first event ALSO gets position 1 (different database)
curl -X POST http://localhost:8080/events \
  -H "X-API-Key: bob-secret-key-456" \
  -d '...'
# Response: {"position":1,...}
```

**Queries are Isolated:**
```bash
# Alice can only see her events
curl -H "X-API-Key: alice-secret-key-123" \
  "http://localhost:8080/events?from=1"
# Returns only Alice's events

# Bob can only see his events
curl -H "X-API-Key: bob-secret-key-456" \
  "http://localhost:8080/events?from=1"
# Returns only Bob's events
```

## Integration with ebu Library

Each tenant can use the ebu library independently:

```go
// Alice's application
aliceStore := client.NewEventStoreAdapter(
    "http://localhost:8080",
    "alice-secret-key-123",
)
aliceBus := eventbus.New(eventbus.WithStore(aliceStore))

// Bob's application
bobStore := client.NewEventStoreAdapter(
    "http://localhost:8080",
    "bob-secret-key-456",
)
bobBus := eventbus.New(eventbus.WithStore(bobStore))

// Events are completely isolated
eventbus.Publish(aliceBus, OrderPlaced{...})  // Goes to alice.db
eventbus.Publish(bobBus, OrderPlaced{...})    // Goes to bob.db
```

## Use Cases

### 1. SaaS Applications

Perfect for multi-customer SaaS:
```yaml
tenants:
  - name: "customer-acme-corp"
    api_key: "acme-prod-key-very-long-random-string"
  - name: "customer-globex"
    api_key: "globex-prod-key-very-long-random-string"
```

### 2. Multi-Environment

Separate dev/staging/prod on one server:
```yaml
tenants:
  - name: "dev"
    api_key: "dev-key-123"
  - name: "staging"
    api_key: "staging-key-456"
  - name: "production"
    api_key: "prod-key-789"
```

### 3. Team Collaboration

Isolated streams per team:
```yaml
tenants:
  - name: "team-frontend"
    api_key: "frontend-team-key"
  - name: "team-backend"
    api_key: "backend-team-key"
  - name: "team-mobile"
    api_key: "mobile-team-key"
```

### 4. Multi-Project

Different projects, one server:
```yaml
tenants:
  - name: "project-alpha"
    api_key: "alpha-key"
  - name: "project-beta"
    api_key: "beta-key"
```

## Management Endpoints

### List All Tenants

```bash
curl -H "X-API-Key: any-valid-key" http://localhost:8080/tenants
```

Response:
```json
{
  "tenants": ["alice", "bob", "charlie"],
  "count": 3
}
```

### Per-Tenant Metrics

```bash
curl -H "X-API-Key: alice-secret-key-123" http://localhost:8080/metrics
```

Response:
```json
{
  "tenant": "alice",
  "total_events": 42,
  "timestamp": 1759632128
}
```

## Security Considerations

1. **API Key Strength**: Use long, random keys (32+ characters)
   ```yaml
   api_key: "prod-alice-8f4e9a2b5c1d6e3f7a8b9c0d1e2f3a4b5c6d7e8f9a0b"
   ```

2. **Key Rotation**: Update `tenants.yaml` and restart server

3. **File Permissions**: Protect your config file
   ```bash
   chmod 600 tenants.yaml
   ```

4. **Database Backups**: Back up entire `data/` directory
   ```bash
   tar -czf tenants-backup.tar.gz data/
   ```

## Adding New Tenants

1. Stop the server
2. Add tenant to `tenants.yaml`:
   ```yaml
   - name: "new-customer"
     api_key: "new-customer-key"
   ```
3. Restart the server
4. New database will be created automatically

## Removing Tenants

1. Stop the server
2. Remove tenant from `tenants.yaml`
3. Optionally, delete database: `rm data/tenant-name.db*`
4. Restart the server

## Performance Notes

- Each tenant database is independent
- No cross-tenant locking or contention
- Performance scales linearly with tenant count
- Recommended: < 100 tenants per server for optimal performance
- For > 100 tenants, consider multiple server instances

## Monitoring

Monitor per-tenant metrics:

```bash
# Check all tenant metrics
for key in alice-key bob-key charlie-key; do
  echo "Tenant metrics for $key:"
  curl -s -H "X-API-Key: $key" http://localhost:8080/metrics | jq .
done
```

## Troubleshooting

**Invalid API Key:**
```
curl -H "X-API-Key: wrong-key" http://localhost:8080/metrics
# 401 Unauthorized: Invalid API key
```

**Missing Config File:**
```
./ebus -config missing.yaml
# Failed to load tenants config: read config file: no such file or directory
```

**Duplicate API Keys:**
```yaml
tenants:
  - name: "alice"
    api_key: "same-key"
  - name: "bob"
    api_key: "same-key"  # ERROR!
```
Server will fail to start with: `duplicate API key for tenant: bob`

## Migration from Single-Tenant

To migrate from single-tenant to multi-tenant:

1. **Backup** your current database
2. Create `tenants.yaml` with your existing setup:
   ```yaml
   data_dir: "."
   tenants:
     - name: "main"
       api_key: "your-existing-api-key"
   ```
3. Rename your database: `mv events.db main.db`
4. Start with: `./ebus -config tenants.yaml`

## Best Practices

1. **Use descriptive tenant names**: `customer-acme-prod` instead of `tenant1`
2. **Document API keys**: Keep a secure registry of which key belongs to which customer
3. **Monitor disk usage**: Each tenant's database grows independently
4. **Regular backups**: Backup the entire `data/` directory
5. **Log tenant activity**: Monitor which tenants are active
6. **Set up alerts**: Alert when a tenant's database grows unexpectedly

## Conclusion

Multi-tenant mode provides enterprise-grade data isolation while maintaining the simplicity and performance of ebus. Perfect for SaaS applications, managed services, and any scenario requiring strict data separation.
