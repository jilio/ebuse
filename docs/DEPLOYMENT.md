# Deployment Guide

## Railway / Nixpacks Deployment

### Required Configuration

**1. Mount your `tenants.yaml` config file:**

Location: `/etc/ebuse/tenants.yaml`

Example config:

```yaml
data_dir: "/data"

tenants:
  - name: "tenant-a"
    api_key: "your-secret-key-a"

  - name: "tenant-b"
    api_key: "your-secret-key-b"
```

**2. Mount persistent volume for data:**

Location: `/data`

This directory will store all tenant SQLite databases:

- `/data/tenant-a.db`
- `/data/tenant-b.db`

### Railway Setup

1. **Create a new service** from this repository

2. **Add a volume:**
   - Mount path: `/data`
   - This persists your databases across redeploys

3. **Add a config file:**
   - Mount your `tenants.yaml` at: `/etc/ebuse/tenants.yaml`
   - Or use Railway's "Config as Code" feature

4. **Optional environment variables:**

   ```
   PORT=8080              # Default: 8080
   RATE_LIMIT=100         # Requests per second per IP (default: 100)
   RATE_BURST=200         # Burst size (default: 200)
   ENABLE_GZIP=true       # Enable compression (default: true)
   READ_TIMEOUT=30s       # HTTP read timeout (default: 30s)
   WRITE_TIMEOUT=60s      # HTTP write timeout (default: 60s)
   ```

5. **Deploy!** Railway will automatically:
   - Build the Go binary
   - Start the server with your config
   - Persist data in `/data` across redeploys

### File Locations Summary

| What | Path | Purpose |
|------|------|---------|
| Config file | `/etc/ebuse/tenants.yaml` | Tenant API keys and settings |
| Data directory | `/data` | SQLite databases (persistent) |
| Database files | `/data/{tenant-name}.db` | Per-tenant event storage |

### Health Check

Add a health check endpoint to your deployment:

- URL: `http://your-app/health`
- Expected response: `{"status":"healthy"}`

### Example tenants.yaml

```yaml
# Production configuration
data_dir: "/data"

tenants:
  - name: "production"
    api_key: "prod-key-very-long-random-string"

  - name: "staging"
    api_key: "staging-key-very-long-random-string"

  - name: "development"
    api_key: "dev-key-very-long-random-string"
```

**Security Notes:**

- Use long, random API keys (at least 32 characters)
- Never commit real API keys to git
- Use environment variables or secret management for keys
- Consider rotating keys periodically

## Alternative: Single-Tenant Mode

If you only need one database, you can skip the config file and use environment variables:

```bash
API_KEY=your-secret-key
DB_PATH=/data/events.db
```

Then update `nixpacks.toml`:

```toml
[start]
cmd = "./ebuse"
```

Mount `/data` as persistent volume for the database.
