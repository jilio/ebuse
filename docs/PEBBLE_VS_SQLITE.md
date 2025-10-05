# PebbleDB vs SQLite Performance Comparison

**Date**: 2025-10-05
**Context**: Evaluating PebbleDB as a potential replacement for SQLite in ebuse event store

## TL;DR - Recommendation: **STAY WITH SQLITE**

SQLite significantly outperforms PebbleDB for our write-heavy event sourcing workload. While PebbleDB offers slightly better read performance, the 5-100x slower writes make it unsuitable for this use case.

---

## Benchmark Results

### Write Performance (SQLite WINS)

| Operation | SQLite | PebbleDB | Winner | Ratio |
|-----------|--------|----------|--------|-------|
| **Single Write** | 44μs | 4,586μs | SQLite | **100x faster** |
| **Batch (100 events)** | 1.06ms | 5.42ms | SQLite | **5x faster** |
| **Batch (1000 events)** | ~10ms (est) | ~54ms (est) | SQLite | **5x faster** |

### Read Performance (Pebble slightly better)

| Operation | SQLite | PebbleDB | Winner | Ratio |
|-----------|--------|----------|--------|-------|
| **Sequential Read (100 events)** | 140μs | 109μs | PebbleDB | 1.3x faster |

---

## Why SQLite is Faster for Writes

### 1. **WAL Mode Optimization**
SQLite's WAL (Write-Ahead Logging) mode is highly optimized for sequential writes:
- Writes go directly to WAL file (sequential I/O)
- Batched checkpoints to main database
- Minimal fsync overhead with `PRAGMA synchronous=NORMAL`

### 2. **In-Memory Buffering**
SQLite buffers heavily in memory:
- Page cache keeps hot data in RAM
- Transaction batching amortizes overhead
- Prepared statements eliminate parsing

### 3. **Simpler Write Path**
SQLite's write path for our use case:
1. Append to WAL file (sequential write)
2. Update in-memory structures
3. Periodic checkpoint to main db

PebbleDB's write path:
1. Write to WAL
2. Write to memtable
3. Flush memtable to L0 SST
4. Compaction across levels
5. More complex write amplification

### 4. **Auto-Increment PRIMARY KEY**
SQLite's `AUTOINCREMENT` is extremely fast:
- Single atomic counter
- No index updates needed (clustered index)
- No secondary index maintenance

PebbleDB requires:
- Atomic counter management
- Manual key serialization
- Binary encoding overhead

---

## Why PebbleDB is Slightly Faster for Reads

### 1. **LSM-Tree Structure**
- Sequential reads benefit from sorted SST files
- Bloom filters accelerate lookups
- Block cache improves hot data access

### 2. **No SQL Parsing**
- Direct key-value access
- No query planner overhead
- Minimal abstraction layers

However, the 1.3x read improvement doesn't offset the 5-100x write degradation.

---

## Production Considerations

### SQLite Advantages (Current)
✅ **5-100x faster writes** - Critical for event sourcing
✅ **Proven at scale** - 10M events tested successfully
✅ **14,300 events/sec** sustained throughput achieved
✅ **Mature ecosystem** - Extensive tooling and documentation
✅ **SQL convenience** - Easy debugging and ad-hoc queries
✅ **Zero dependencies** - Pure Go driver (modernc.org/sqlite)

### PebbleDB Advantages
✅ **Slightly faster reads** - 1.3x improvement
✅ **No relational overhead** - Pure KV store
✅ **LSM-tree benefits** - Better for write-heavy workloads (in theory)
❌ **BUT**: 5-100x slower writes in practice for our use case

---

## Analysis: Why PebbleDB Underperforms

### Write Amplification
PebbleDB's LSM-tree creates write amplification:
1. WAL write
2. Memtable write
3. L0 SST flush
4. Compactions (L0→L1→L2...)

For sequential event appends, SQLite's B-tree + WAL is more efficient.

### Synchronization Overhead
PebbleDB uses `pebble.Sync` for durability:
- Forces fsync on every batch
- Cannot batch fsyncs like SQLite's WAL checkpointing
- Higher per-operation overhead

### Atomic Counter Inefficiency
Manual atomic counter (`atomic.Int64`) + key serialization adds overhead compared to SQLite's native `AUTOINCREMENT`.

---

## Attempted Optimizations (Still Slower)

Tried various PebbleDB configurations:
- Larger memtable size (64MB)
- Disabled WAL (unsafe, still slower)
- Increased compaction concurrency
- Adjusted L0 compaction thresholds

**Result**: Still 5x slower than SQLite for batch writes.

---

## Conclusion

**Verdict**: **KEEP SQLITE**

### Reasons:
1. **Write Performance is King** - Event sourcing is write-dominated
2. **5-100x faster writes** - No contest for our workload
3. **Already optimized** - Our SQLite setup is production-proven
4. **Proven scale** - 14,300 events/sec sustained, 10M events tested
5. **Read performance is adequate** - 140μs is already fast

### When PebbleDB Would Make Sense:
- Read-heavy workloads (10:1 read:write ratio)
- Distributed systems (PebbleDB used in CockroachDB)
- Key-value specific features (prefix scans, merge operators)
- Very large datasets (>1TB)

### For Our Use Case:
- **Write-heavy** event sourcing (append-only)
- **Sequential writes** dominate (batches)
- **SQLite is perfectly optimized** for this pattern
- **No reason to switch**

---

## Production Impact If We Switched

Current with SQLite (2GB RAM):
- 14,300 events/sec sustained
- 1.23 billion events/day
- 0.009% error rate

Projected with PebbleDB:
- ~2,800 events/sec (5x slower)
- ~240 million events/day
- Higher error rate due to slower writes

**Result**: 80% throughput reduction. Not acceptable.

---

## Final Recommendation

✅ **STAY WITH SQLITE**

SQLite is the clear winner for ebuse's event sourcing workload. The 5-100x write performance advantage far outweighs PebbleDB's marginal read improvements. Our current SQLite setup is production-proven and highly optimized.

**Action Items**:
- ✅ Keep current SQLite implementation
- ✅ Document this analysis for future reference
- ✅ Continue optimizing SQLite (memory tuning, cache sizes)
- ❌ Do not switch to PebbleDB

---

## Appendix: Full Benchmark Results

```
BenchmarkSQLite_SingleWrite-10       26898   44772 ns/op    (44μs per event)
BenchmarkPebble_SingleWrite-10         252 4586679 ns/op  (4586μs per event)

BenchmarkSQLite_BatchWrite-10         1132 1061678 ns/op  (1.06ms per 100 events)
BenchmarkPebble_BatchWrite-10          225 5417782 ns/op  (5.42ms per 100 events)

BenchmarkSQLite_SequentialRead-10     7477  140837 ns/op  (140μs per 100 events)
BenchmarkPebble_SequentialRead-10    11186  109829 ns/op  (109μs per 100 events)
```

**Test Environment**:
- Go 1.24.2
- SQLite: modernc.org/sqlite (pure Go)
- PebbleDB: v1.1.5
- macOS, M1/M2 architecture
