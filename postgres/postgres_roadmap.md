# 🐘 PostgreSQL: Basics → Legend Roadmap

> A backend developer's complete path to mastering PostgreSQL — from your first `SELECT` to reading the source code.

---

## How to Use This Roadmap

Each level builds on the previous. Don't skip levels. **Practice every concept** in a local Postgres instance before moving on. Resources are listed at the bottom.

---

## Level 1 — The Foundation

> *Goal: Be comfortable creating databases, tables, and running basic queries.*

### Core Concepts
- [ ] What is PostgreSQL? (ORDBMS, ACID, MVCC overview)
- [ ] Installing Postgres (native / Docker) & connecting via `psql`
- [ ] `psql` meta-commands: `\l`, `\dt`, `\d`, `\c`, `\x`, `\timing`
- [ ] `CREATE DATABASE`, `DROP DATABASE`
- [ ] Data types: `INTEGER`, `BIGINT`, `SERIAL`, `TEXT`, `VARCHAR(n)`, `BOOLEAN`, `DATE`, `TIMESTAMP`, `TIMESTAMPTZ`, `UUID`, `JSONB`
- [ ] `CREATE TABLE`, `ALTER TABLE`, `DROP TABLE`
- [ ] Constraints: `PRIMARY KEY`, `NOT NULL`, `UNIQUE`, `CHECK`, `DEFAULT`

### SQL Essentials
- [ ] `INSERT`, `UPDATE`, `DELETE`, `UPSERT` (`ON CONFLICT`)
- [ ] `SELECT`, `WHERE`, `ORDER BY`, `LIMIT`, `OFFSET`
- [ ] `LIKE`, `ILIKE`, `IN`, `BETWEEN`, `IS NULL`
- [ ] Aggregate functions: `COUNT`, `SUM`, `AVG`, `MIN`, `MAX`
- [ ] `GROUP BY`, `HAVING`
- [ ] `DISTINCT`, `DISTINCT ON`
- [ ] Type casting with `::` and `CAST()`

> [!TIP]
> Always use `TIMESTAMPTZ` over `TIMESTAMP`. Postgres stores it as UTC internally and converts on display using the session timezone — this saves you from timezone bugs.

---

## Level 2 — Relationships & Joins

> *Goal: Model real-world data with proper relationships and join them fluently.*

- [ ] Foreign Keys (`REFERENCES`, `ON DELETE CASCADE / SET NULL / RESTRICT`)
- [ ] One-to-One, One-to-Many, Many-to-Many relationships
- [ ] Junction / bridge tables for M:N
- [ ] `INNER JOIN`, `LEFT JOIN`, `RIGHT JOIN`, `FULL OUTER JOIN`
- [ ] `CROSS JOIN`, `NATURAL JOIN` (and why to avoid `NATURAL JOIN`)
- [ ] Self joins
- [ ] Subqueries: scalar, row, table subqueries
- [ ] `EXISTS`, `NOT EXISTS`, `ANY`, `ALL`
- [ ] Correlated vs non-correlated subqueries

> [!IMPORTANT]
> **Think in sets, not loops.** SQL is a declarative, set-based language. If you find yourself thinking "for each row, do X", you're likely writing it wrong. Think "what set of rows do I need?".

---

## Level 3 — Schema Design & Normalization

> *Goal: Design schemas that are correct, efficient, and maintainable.*

- [ ] Normalization: 1NF → 2NF → 3NF → BCNF (with examples)
- [ ] When and why to **denormalize** (read-heavy workloads, caching tables)
- [ ] Surrogate keys (`SERIAL`, `UUID`) vs natural keys — tradeoffs
- [ ] Naming conventions: `snake_case`, singular table names, `_id` suffix
- [ ] `ENUM` types via `CREATE TYPE ... AS ENUM`
- [ ] Domain types: `CREATE DOMAIN`
- [ ] Schemas as namespaces (`CREATE SCHEMA`, `search_path`)
- [ ] Soft deletes (`deleted_at` column) vs hard deletes — tradeoffs
- [ ] Audit columns: `created_at`, `updated_at` (with triggers)

> [!TIP]
> **UUIDs as PKs** are great for distributed systems but cause B-tree index fragmentation due to randomness. Use `uuid_generate_v7()` (time-ordered UUIDs, PG 17+) or `ULID` to get both uniqueness and sequential ordering.

---

## Level 4 — Indexing Deep Dive

> *Goal: Understand how indexes work internally and when to use each type.*

### Index Types
- [ ] **B-tree** (default) — equality + range queries. Understand the tree structure.
- [ ] **Hash** — equality only, smaller than B-tree but limited
- [ ] **GIN** (Generalized Inverted Index) — for `JSONB`, arrays, full-text search
- [ ] **GiST** (Generalized Search Tree) — geometric, range types, full-text
- [ ] **BRIN** (Block Range Index) — massive tables with naturally ordered data (e.g., time-series `created_at`)
- [ ] **SP-GiST** — space-partitioned data (IP addresses, phone numbers)

### Index Features
- [ ] Partial indexes: `CREATE INDEX ... WHERE condition`
- [ ] Expression indexes: `CREATE INDEX ... ON (LOWER(email))`
- [ ] Covering indexes: `INCLUDE` clause (index-only scans)
- [ ] Multicolumn indexes & column ordering (leftmost prefix rule)
- [ ] `UNIQUE INDEX` vs `UNIQUE CONSTRAINT`
- [ ] `CONCURRENTLY` — creating indexes without locking writes

### Internals & Maintenance
- [ ] Index bloat & `REINDEX`
- [ ] `pg_stat_user_indexes` — find unused indexes
- [ ] Cost of indexes: write amplification, storage overhead
- [ ] When NOT to index (small tables, low selectivity columns)

> [!CAUTION]
> **Every index you add slows down writes** (`INSERT`, `UPDATE`, `DELETE`). A table with 10 indexes means every insert updates 10 B-trees. Profile before adding indexes blindly.

---

## Level 5 — Query Optimization & `EXPLAIN`

> *Goal: Read execution plans like a pro and make slow queries fast.*

### `EXPLAIN` Mastery
- [ ] `EXPLAIN` vs `EXPLAIN ANALYZE` vs `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`
- [ ] Reading plan nodes: `Seq Scan`, `Index Scan`, `Index Only Scan`, `Bitmap Scan`
- [ ] `Nested Loop`, `Hash Join`, `Merge Join` — when each is chosen
- [ ] `Sort`, `HashAggregate`, `GroupAggregate`
- [ ] Understanding `cost`, `rows`, `width`, `actual time`, `loops`
- [ ] `Buffers: shared hit / read` — cache hit ratio per node

### Optimization Techniques
- [ ] Fix bad cardinality estimates (stale stats → run `ANALYZE`)
- [ ] The planner and `random_page_cost`, `effective_cache_size`, `work_mem`
- [ ] CTEs: pre-PG12 are optimization fences; PG12+ can be inlined (`MATERIALIZED` / `NOT MATERIALIZED`)
- [ ] `LIMIT` push-down optimization
- [ ] Pagination: **keyset pagination** (`WHERE id > last_seen_id ORDER BY id LIMIT N`) vs `OFFSET` (never use `OFFSET` for deep pages)
- [ ] Avoiding `SELECT *` — only select what you need
- [ ] `JOIN` ordering — Postgres reorders joins, but excessive joins can blow up plan search space (`join_collapse_limit`)

> [!TIP]
> Use **[explain.dalibo.com](https://explain.dalibo.com)** to visualize `EXPLAIN` output. Paste the JSON format for the best results.

---

## Level 6 — Transactions, Concurrency & MVCC

> *Goal: Deeply understand how Postgres handles concurrent access without locking everything.*

### Transactions
- [ ] `BEGIN`, `COMMIT`, `ROLLBACK`, `SAVEPOINT`, `ROLLBACK TO SAVEPOINT`
- [ ] Autocommit behavior in `psql` and application drivers
- [ ] Isolation levels: `READ COMMITTED` (default), `REPEATABLE READ`, `SERIALIZABLE`
- [ ] Phantom reads, non-repeatable reads, dirty reads — which levels prevent which

### MVCC Internals 🔥
- [ ] Every row has hidden system columns: `xmin`, `xmax`, `ctid`
- [ ] `xmin` = transaction ID that inserted the row
- [ ] `xmax` = transaction ID that deleted/updated the row (0 if alive)
- [ ] `ctid` = physical location `(page, offset)` — changes on `UPDATE` (not in-place!)
- [ ] **Visibility rules**: a row is visible to txn T if `xmin` is committed AND (`xmax` is 0 OR `xmax` is aborted OR `xmax > T`)
- [ ] Snapshot isolation — each transaction gets a snapshot of which txns are committed
- [ ] `pg_xact` (formerly `pg_clog`) — commit status of transactions

### Locking
- [ ] Row-level locks: `FOR UPDATE`, `FOR SHARE`, `FOR NO KEY UPDATE`, `FOR KEY SHARE`
- [ ] Table-level locks: `ACCESS SHARE` → `ACCESS EXCLUSIVE` (8 levels)
- [ ] Advisory locks: application-level locking (`pg_advisory_lock`)
- [ ] Deadlock detection (`deadlock_timeout`, auto-detection)
- [ ] `pg_locks` view — diagnosing lock contention

> [!IMPORTANT]
> **Postgres `UPDATE` does NOT modify in place.** It marks the old row as dead (`xmax = current_txn`) and inserts a new version. This is why `VACUUM` exists — to reclaim dead rows.

---

## Level 7 — VACUUM, Autovacuum & Storage

> *Goal: Understand Postgres storage internals and keep your database healthy.*

### VACUUM
- [ ] Why VACUUM exists: reclaiming dead tuples from MVCC
- [ ] `VACUUM` (standard) vs `VACUUM FULL` (rewrites entire table, exclusive lock!)
- [ ] `VACUUM ANALYZE` — reclaims + updates planner statistics
- [ ] Visibility Map (VM) — tracks all-visible pages for index-only scans
- [ ] Free Space Map (FSM) — tracks available space for new inserts

### Autovacuum Tuning
- [ ] How autovacuum decides when to run: `autovacuum_vacuum_threshold` + `autovacuum_vacuum_scale_factor` × row count
- [ ] Per-table tuning: `ALTER TABLE ... SET (autovacuum_vacuum_scale_factor = 0.01)`
- [ ] `autovacuum_max_workers`, `autovacuum_naptime`
- [ ] `autovacuum_vacuum_cost_delay` & `autovacuum_vacuum_cost_limit` — throttling
- [ ] Monitoring: `pg_stat_user_tables` → `n_dead_tup`, `last_autovacuum`

### Storage Internals
- [ ] **Pages** (8KB blocks) — the fundamental I/O unit
- [ ] **Heap** — the main table data file, an array of pages
- [ ] **TOAST** (The Oversized-Attribute Storage Technique) — for values >2KB, compressed/out-of-line stored
- [ ] **Tablespaces** — placing tables/indexes on different disks
- [ ] `pg_relation_filepath()` — find the actual file for a table
- [ ] File layout on disk: `base/<oid>/<relfilenode>`, segment files (1GB chunks)

### Transaction ID Wraparound ⚠️
- [ ] 32-bit transaction IDs → ~4 billion transactions before wraparound
- [ ] `VACUUM FREEZE` — marks old rows as "frozen" (visible to all future txns)
- [ ] `autovacuum_freeze_max_age` — forces vacuum before wraparound
- [ ] **If this fails → database shuts down to prevent data corruption**

> [!CAUTION]
> **Transaction ID wraparound is a production killer.** If autovacuum falls behind, Postgres will refuse new transactions. Monitor `age(datfrozenxid)` and alert if it exceeds 500M.

---

## Level 8 — Advanced SQL & Features

> *Goal: Use advanced Postgres features that set it apart from other databases.*

### Window Functions
- [ ] `ROW_NUMBER()`, `RANK()`, `DENSE_RANK()`, `NTILE()`
- [ ] `LEAD()`, `LAG()`, `FIRST_VALUE()`, `LAST_VALUE()`, `NTH_VALUE()`
- [ ] `SUM() OVER (ORDER BY ...)` — running totals
- [ ] `PARTITION BY` — windowing within groups
- [ ] Frame clauses: `ROWS BETWEEN`, `RANGE BETWEEN`, `GROUPS BETWEEN`

### CTEs & Recursive Queries
- [ ] `WITH` (CTEs) for readable complex queries
- [ ] `WITH RECURSIVE` for hierarchical data (org charts, categories, graphs)
- [ ] Cycle detection in recursive CTEs (`CYCLE` clause, PG14+)

### JSON & JSONB
- [ ] `JSONB` vs `JSON` (binary vs text, `JSONB` is almost always better)
- [ ] Operators: `->`, `->>`, `#>`, `#>>`, `@>`, `?`, `?|`, `?&`
- [ ] `jsonb_each`, `jsonb_array_elements`, `jsonb_to_record`
- [ ] Indexing JSONB: GIN index on entire column or specific paths (`jsonb_path_ops`)
- [ ] `jsonb_set`, `jsonb_insert`, `||` (merge)

### Full-Text Search
- [ ] `tsvector`, `tsquery`, `to_tsvector()`, `to_tsquery()`, `plainto_tsquery()`
- [ ] `@@` match operator
- [ ] GIN indexes on `tsvector` columns
- [ ] Ranking: `ts_rank()`, `ts_rank_cd()`
- [ ] Custom dictionaries, stop words, configurations

### Other Powerful Features
- [ ] `GENERATED ALWAYS AS` — stored computed columns
- [ ] `LATERAL` joins — for correlated subqueries as join sources
- [ ] `FILTER` clause on aggregates: `COUNT(*) FILTER (WHERE status = 'active')`
- [ ] `GROUPING SETS`, `ROLLUP`, `CUBE` for multi-dimensional aggregation
- [ ] Array operations: `ANY()`, `ALL()`, `array_agg()`, `unnest()`
- [ ] Range types: `int4range`, `tstzrange`, `&&` (overlap), `@>` (contains)

---

## Level 9 — Production Postgres

> *Goal: Run Postgres reliably in production at scale.*

### Configuration Tuning
- [ ] `shared_buffers` (~25% of RAM)
- [ ] `effective_cache_size` (~75% of RAM, hint to planner)
- [ ] `work_mem` (per-sort/hash, be careful with high values × concurrent queries)
- [ ] `maintenance_work_mem` (for `VACUUM`, `CREATE INDEX`, etc.)
- [ ] `wal_buffers`, `checkpoint_completion_target`
- [ ] `max_connections` vs connection pooling (use **PgBouncer**)
- [ ] Use **[pgtune.leopard.in.ua](https://pgtune.leopard.in.ua)** as a starting point

### WAL (Write-Ahead Log)
- [ ] Every change is written to WAL **before** the data file — crash recovery guarantee
- [ ] WAL segments (16MB files by default)
- [ ] `Checkpoints` — flush dirty pages from shared buffers to disk, advance WAL
- [ ] `archive_mode` & `archive_command` — for PITR and backups
- [ ] `wal_level`: `minimal`, `replica`, `logical`

### Backup & Recovery
- [ ] `pg_dump` / `pg_dumpall` — logical backups (SQL or custom format)
- [ ] `pg_basebackup` — physical backups (full filesystem copy)
- [ ] **PITR** (Point-In-Time Recovery) — restore to any second using base backup + WAL archives
- [ ] `pg_restore` — restore from custom-format dumps
- [ ] Tools: **pgBackRest**, **Barman**, **WAL-G**

### Replication
- [ ] **Streaming Replication** — physical, byte-for-byte copy of primary
- [ ] `primary_conninfo`, `pg_hba.conf` for replication
- [ ] Synchronous vs asynchronous replication
- [ ] **Hot standby** — read queries on replicas
- [ ] `pg_stat_replication` — monitor replication lag
- [ ] **Logical Replication** — table-level, cross-version, selective
  - [ ] `PUBLICATION`, `SUBSCRIPTION`
  - [ ] Use cases: zero-downtime upgrades, selective sync, cross-cluster

### Partitioning
- [ ] **Declarative partitioning** (PG10+): `RANGE`, `LIST`, `HASH`
- [ ] Partition pruning — planner skips irrelevant partitions
- [ ] Sub-partitioning
- [ ] Partition maintenance: adding/dropping/detaching partitions
- [ ] Indexes on partitioned tables (per-partition vs global)
- [ ] When to partition: tables >100GB or time-series data

### Connection Management
- [ ] Why `max_connections = 1000` is a terrible idea (context switching, memory)
- [ ] **PgBouncer**: transaction pooling vs session pooling vs statement pooling
- [ ] **pgcat**, **Odyssey** — modern alternatives
- [ ] Connection lifecycle: backend process per connection, `fork()` cost

### Monitoring & Observability
- [ ] `pg_stat_activity` — active queries, waiting states, pids
- [ ] `pg_stat_statements` — query-level performance stats (top N slow queries)
- [ ] `pg_stat_user_tables` — seq scans, index scans, dead tuples, vacuum stats
- [ ] `pg_stat_bgwriter` — checkpoint and background writer stats
- [ ] `pg_stat_io` (PG16+) — I/O statistics
- [ ] Log analysis: `log_min_duration_statement`, `auto_explain`
- [ ] Tools: **pgMonitor**, **Prometheus + postgres_exporter + Grafana**, **pgwatch2**

> [!TIP]
> **The single most impactful tool** you can enable is `pg_stat_statements`. It aggregates query stats (calls, total_time, mean_time, rows) and instantly shows you your slowest queries. Enable it day one.

---

## Level 10 — Legend Level: Internals & Source Code

> *Goal: Understand how Postgres works under the hood. Read the source. Contribute.*

### Architecture Overview
```
Client → Postmaster (listener) → Fork → Backend Process (one per connection)
                                          ↓
                            Parser → Analyzer → Rewriter → Planner → Executor
                                                                        ↓
                                                              Buffer Manager ↔ Disk
                                                              WAL Writer ↔ WAL files
```

### Process Model
- [ ] **Postmaster** — the main daemon, listens on port 5432, forks backends
- [ ] **Backend process** — one per connection, handles parsing → execution
- [ ] **Background workers**: autovacuum launcher, WAL writer, checkpointer, stats collector, logical replication workers, background workers (custom extensions)
- [ ] Shared memory architecture — `shared_buffers`, lock tables, proc array

### Buffer Manager & Shared Buffers
- [ ] **Buffer pool** — in-memory cache of 8KB pages from disk
- [ ] **Clock-sweep** eviction algorithm (not LRU!)
- [ ] Buffer tags: `(RelFileNode, ForkNumber, BlockNumber)` → buffer ID
- [ ] Pin/unpin mechanism — prevents eviction while in use
- [ ] Dirty page tracking and write-back via checkpointer/bgwriter

### Query Processing Pipeline
| Stage | What it does |
|---|---|
| **Parser** | SQL text → raw parse tree (gram.y, scan.l) |
| **Analyzer** | Resolve names, types → Query tree |
| **Rewriter** | Apply rules (views are rules!) → Rewritten query |
| **Planner/Optimizer** | Cost-based optimization → Plan tree |
| **Executor** | Volcano-model (iterator) → Execute node by node |

- [ ] **Cost model**: `startup_cost` + `run_cost`, based on table statistics (`pg_statistic`)
- [ ] **Statistics collector**: histogram buckets, MCV (Most Common Values), NDV, correlation
- [ ] `pg_statistic` and `pg_stats` — where the planner gets its numbers
- [ ] Custom statistics: `CREATE STATISTICS` for multi-column correlation

### HOT (Heap-Only Tuples)
- [ ] If an `UPDATE` doesn't modify any indexed column, Postgres can chain the new version from the old version **within the same page** — no index update needed!
- [ ] Dramatically reduces index bloat for update-heavy workloads
- [ ] Check with `pg_stat_user_tables.n_tup_hot_upd`
- [ ] Requires free space on the same page (keep fillfactor < 100: `ALTER TABLE SET (fillfactor = 80)`)

### The Catalog
- [ ] Everything in Postgres is stored in **system catalog tables** (`pg_class`, `pg_attribute`, `pg_type`, `pg_proc`, `pg_namespace`, `pg_index`, etc.)
- [ ] `pg_class` — every table, index, view, sequence, etc. is a row here
- [ ] `pg_attribute` — every column of every table
- [ ] The catalog **is** the metadata. Postgres eats its own dog food.

### Extensions & Hooks
- [ ] Extension framework: `CREATE EXTENSION`
- [ ] Must-know extensions:
  - `pg_stat_statements` — query performance
  - `pg_trgm` — trigram-based text similarity and indexing
  - `postgis` — geospatial data
  - `pgcrypto` — encryption
  - `pg_partman` — automated partition management
  - `timescaledb` — time-series
  - `citus` — distributed Postgres
  - `pgvector` — vector similarity search (AI/ML embeddings)
- [ ] **Hooks** — extension points in the backend (e.g., `planner_hook`, `executor_hook`, `ProcessUtility_hook`) — this is how extensions modify Postgres behavior without patching source

### Source Code Orientation
- [ ] Repo: [github.com/postgres/postgres](https://github.com/postgres/postgres)
- [ ] Key directories:

| Directory | Contains |
|---|---|
| `src/backend/parser/` | SQL parser (gram.y, scan.l) |
| `src/backend/optimizer/` | Query planner and optimizer |
| `src/backend/executor/` | Executor nodes |
| `src/backend/access/heap/` | Heap AM (table storage) |
| `src/backend/access/nbtree/` | B-tree index implementation |
| `src/backend/storage/buffer/` | Buffer manager |
| `src/backend/storage/lmgr/` | Lock manager |
| `src/backend/utils/cache/` | System catalog caches |
| `src/include/` | All header files |

> [!NOTE]
> The Postgres source is remarkably well-commented C code. Start by reading `src/backend/access/heap/README` and `src/backend/optimizer/README` — they are excellent design documents written by the core developers.

---

## 🏋️ Practice Projects by Level

| Level | Project |
|---|---|
| 1-2 | Build a **library management system** (books, authors, borrowers, loans) |
| 3-4 | Design an **e-commerce schema** with proper normalization, indexes, and JSONB for product attributes |
| 5-6 | Take a slow API query, run `EXPLAIN ANALYZE`, add indexes, rewrite the query, and measure improvement |
| 7 | Set up autovacuum monitoring, simulate table bloat, and tune autovacuum for a high-write table |
| 8 | Build a **Reddit-like threaded comments** system using `WITH RECURSIVE` and window functions |
| 9 | Set up streaming replication, test failover, configure pgBouncer, and implement PITR |
| 10 | Write a simple **Postgres extension in C** that adds a custom function. Read one executor node's source code end-to-end. |

---

## 📚 Recommended Resources

### Books (in order)
1. **"Learn PostgreSQL"** — Luca Ferrari & Enrico Pirozzi *(beginner → intermediate)*
2. **"PostgreSQL 14 Internals"** — Egor Rogov *(free! best internals book, covers MVCC, WAL, query processing)* — [postgrespro.com/community/books/internals](https://postgrespro.com/community/books/internals)
3. **"The Art of PostgreSQL"** — Dimitri Fontaine *(advanced SQL mastery)*
4. **"Mastering PostgreSQL in Application Development"** — Dimitri Fontaine

### Online
- [Official Docs](https://www.postgresql.org/docs/current/) — The **best** database documentation ever written. Read it.
- [pgexercises.com](https://pgexercises.com) — Interactive SQL exercises
- [use-the-index-luke.com](https://use-the-index-luke.com) — Deep dive into SQL indexing
- [Postgres Wiki](https://wiki.postgresql.org) — Internal architecture pages
- [Crunchy Data Blog](https://www.crunchydata.com/blog) — Production Postgres tips
- [pganalyze Blog](https://pganalyze.com/blog) — Performance tuning deep dives
- [Haki Benita's Blog](https://hakibenita.com/) — Exceptional Postgres optimization articles
- [Hussein Nasser's YouTube](https://www.youtube.com/@haborahora) — Great database internals videos
- [CMU Database Course (Andy Pavlo)](https://www.youtube.com/playlist?list=PLSE8ODhjZXjbj8BMuIrRcacnQh20hmY9g) — foundational DB concepts

---

> *"The best way to learn Postgres is to use it to solve real problems, then wonder why it's slow, then read the docs, then realize you were wrong about everything, then read them again."*
