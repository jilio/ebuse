#!/bin/bash

# Load testing script for ebuse
# Requires: go-wrk or wrk installed

set -e

API_URL="${API_URL:-https://ebuse.lookhere.tech}"
API_KEY="${API_KEY:-my-secret-key}"

echo "========================================="
echo "ebuse Load Testing Suite"
echo "========================================="
echo "Target: $API_URL"
echo "API Key: ${API_KEY:0:8}..."
echo ""

# Test 1: GET /position (lightweight read)
echo "Test 1: GET /position (lightweight read)"
echo "-----------------------------------------"
go-wrk -c 50 -d 10 -H "X-API-Key: $API_KEY" "$API_URL/position"
echo ""

# Test 2: POST /events (write operations)
echo "Test 2: POST /events (write operations)"
echo "-----------------------------------------"
cat > /tmp/event.json <<EOF
{"type":"BenchmarkEvent","data":{"message":"load test","timestamp":"$(date -u +%Y-%m-%dT%H:%M:%SZ)"}}
EOF

go-wrk -c 50 -d 10 -M POST \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -body @/tmp/event.json \
  "$API_URL/events"
echo ""

# Test 3: GET /events?from=0 (range read)
echo "Test 3: GET /events?from=0 (range read)"
echo "-----------------------------------------"
go-wrk -c 50 -d 10 -H "X-API-Key: $API_KEY" "$API_URL/events?from=0&to=100"
echo ""

# Test 4: Heavy concurrent writes
echo "Test 4: Heavy concurrent writes (100 connections)"
echo "-----------------------------------------"
go-wrk -c 100 -d 10 -M POST \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -body @/tmp/event.json \
  "$API_URL/events"
echo ""

# Cleanup
rm -f /tmp/event.json

echo "========================================="
echo "Load testing complete!"
echo "========================================="
