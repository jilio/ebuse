# Store Backend Configuration

ebuse supports two storage backends: **SQLite** and **PebbleDB**.

## Default Backend

**PebbleDB** is the default backend (15-33x faster than SQLite for writes).

## Choosing a Backend

### Environment Variable (Single-tenant mode)

```bash
# Use PebbleDB (default)
STORE_BACKEND=pebble ./ebuse

# Use SQLite
STORE_BACKEND=sqlite ./ebuse
```

### YAML Config (Multi-tenant mode)

```yaml
data_dir: "data"
store_backend: "pebble"  # or "sqlite"
tenants:
  - name: tenant1
    api_key: your-api-key-1
  - name: tenant2
    api_key: your-api-key-2
```

## Storage Locations

### SQLite
- Single file per tenant: `data/tenant1.db`
- Easy to backup/copy

### PebbleDB
- Directory per tenant: `data/tenant1/`
- Contains WAL, memtables, and SST files
- Backup requires copying entire directory

## Performance Comparison

| Operation | SQLite | PebbleDB | Winner |
|-----------|--------|----------|--------|
| Single write | 47μs | 1.4μs | **PebbleDB (33x)** |
| Batch (100) | 1.1ms | 73μs | **PebbleDB (15x)** |
| Sequential read | 139μs | 104μs | **PebbleDB (1.3x)** |

See [PEBBLE_VS_SQLITE.md](PEBBLE_VS_SQLITE.md) for detailed analysis.

## When to Use Each Backend

### Use PebbleDB (default) when:
- ✅ Maximum write performance is needed
- ✅ Event sourcing/append-heavy workloads
- ✅ High throughput requirements (>10k events/sec)
- ✅ You don't need SQL queries

### Use SQLite when:
- ✅ You need SQL debugging/analytics
- ✅ Integration with SQL tools
- ✅ Simpler single-file storage
- ✅ Team prefers relational databases

## Migration Between Backends

Both backends implement the same `EventStore` interface, so you can switch between them by changing the configuration. However, data is not automatically migrated.

To migrate data:
1. Export events from old backend
2. Change `store_backend` config
3. Import events to new backend

(Migration tool coming soon)
