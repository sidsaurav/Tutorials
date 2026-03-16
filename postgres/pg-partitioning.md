# PostgreSQL Table Partitioning — A Deep Dive

> **Audience**: Backend engineers (SDE-2+) working with PostgreSQL in production.
> **Sources**: [dyl.dog — Everything You Need to Know About PostgreSQL Partitioning](https://dyl.dog/everything-you-need-to-know-about-postgres-partitioning/), [PostgreSQL 16 Official Docs — Table Partitioning](https://www.postgresql.org/docs/16/ddl-partitioning.html)

---

## Table of Contents

1. [First Principles — The "What" and "Why"](#1-first-principles--the-what-and-why)
2. [How Data Lives on Disk (Mental Model)](#2-how-data-lives-on-disk-mental-model)
3. [The Three Partitioning Strategies](#3-the-three-partitioning-strategies)
4. [Partitioning and Indexing — The Deep Connection](#4-partitioning-and-indexing--the-deep-connection)
5. [Partitioning and Other Database Subsystems](#5-partitioning-and-other-database-subsystems)
6. [Declarative Partitioning Syntax (PostgreSQL 10+)](#6-declarative-partitioning-syntax-postgresql-10)
7. [Real-World Production Implementation](#7-real-world-production-implementation)
8. [Production Gotchas and Pitfalls](#8-production-gotchas-and-pitfalls)
9. [Monitoring and Observability](#9-monitoring-and-observability)
10. [When NOT to Partition](#10-when-not-to-partition)
11. [Quick-Reference Cheat Sheet](#11-quick-reference-cheat-sheet)

---

## 1. First Principles — The "What" and "Why"

### 1.1 What is Table Partitioning?

Partitioning is the process of **splitting one large logical table into multiple smaller physical tables** (called _partitions_) while maintaining transparent access — the application still queries a single table name, and PostgreSQL routes reads and writes to the correct physical partition automatically.

```
┌─────────────────────────────────────────┐
│           invoices  (logical)           │  ← App queries this
├─────────────┬─────────────┬─────────────┤
│ invoices    │ invoices    │ invoices    │  ← PG stores data here
│ _2025_01    │ _2025_02    │ _2025_03    │
│ (physical)  │ (physical)  │ (physical)  │
└─────────────┴─────────────┴─────────────┘
```

Each partition is an independent table with its own:
- **Heap storage** (the actual data pages on disk)
- **Indexes** (each partition has its own B-tree / GIN / etc.)
- **Constraints and defaults**
- **TOAST table** (for large values)
- **Visibility map and free-space map** (for VACUUM)

The parent "partitioned table" is a **shell** — it stores no data itself. It only holds metadata describing how rows are routed to child partitions.

### 1.2 Why Does This Matter?

PostgreSQL stores table data in **8 KB pages** on disk. When a table grows to hundreds of millions of rows, several things break down:

| Problem | Without Partitioning | With Partitioning |
|---|---|---|
| **Index size** | One massive B-tree spanning all rows; upper levels evicted from shared_buffers under memory pressure | Many small B-trees that individually fit in memory |
| **VACUUM** | Single enormous VACUUM pass; can hold locks, bloat pg_xact | Smaller, faster VACUUM per partition; less disruptive |
| **Bulk deletes** | `DELETE FROM t WHERE created_at < '2024-01-01'` → marks rows dead → requires VACUUM → bloat | `DROP TABLE t_2023_12;` or `ALTER TABLE t DETACH PARTITION t_2023_12;` → instant, no VACUUM |
| **Sequential scans** | Scan entire 500 GB table even if you need one month | Scan only the 10 GB monthly partition |
| **Backup/restore** | Must back up the whole table | Can do partition-level pg_dump for selective restore |

### 1.3 The Rule of Thumb

> **Partition when the table size exceeds the physical memory (RAM) of your database server.**
>
> — PostgreSQL official documentation

Below that threshold, indexes and shared_buffers handle things fine. Above it, the benefits of smaller physical units start compounding.

---

## 2. How Data Lives on Disk (Mental Model)

To understand _why_ partitioning helps, you need to understand how PostgreSQL physically stores and retrieves data.

### 2.1 Heap, Pages, and Tuples

```
Table "invoices" on disk
┌────────────────────────────────┐
│ Page 0  [8 KB]                 │  ← header + tuple pointers + tuples
│ Page 1  [8 KB]                 │
│ Page 2  [8 KB]                 │
│ ...                            │
│ Page N  [8 KB]                 │  ← N can be millions for large tables
└────────────────────────────────┘
```

- A **heap** is an unordered collection of pages.
- Each **page** is 8 KB and holds multiple **tuples** (rows).
- To find a specific row, PostgreSQL either:
  - Does a **sequential scan** (reads every page, O(N)), or
  - Uses an **index** to jump to the exact page (O(log N)).

### 2.2 The Problem with Scale

When the table has 1 billion rows:
- The heap is ~100+ GB on disk.
- A B-tree index on `created_at` might be ~20 GB.
- The index's **upper levels** (root + internal pages) need to stay in `shared_buffers` for fast lookups.
- Under memory pressure, those upper-level index pages get **evicted**, turning every index lookup into random disk I/O.

Partitioning shrinks both the heap and indexes **per partition**, making it far more likely that hot data (and its indexes) fit entirely in RAM.

---

## 3. The Three Partitioning Strategies

PostgreSQL supports three declarative partitioning strategies. The choice depends on your data shape and query patterns.

### 3.1 Range Partitioning

Divides data by **non-overlapping value ranges** on the partition key. By far the most common strategy — especially for time-series data.

```sql
CREATE TABLE events (
    id          BIGSERIAL,
    user_id     BIGINT       NOT NULL,
    event_type  TEXT         NOT NULL,
    payload     JSONB,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)          -- partition key MUST be in PK
) PARTITION BY RANGE (created_at);

-- Monthly partitions
CREATE TABLE events_2025_01 PARTITION OF events
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

CREATE TABLE events_2025_02 PARTITION OF events
    FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');

CREATE TABLE events_2025_03 PARTITION OF events
    FOR VALUES FROM ('2025-03-01') TO ('2025-04-01');

-- Safety net
CREATE TABLE events_default PARTITION OF events DEFAULT;
```

**Key properties:**
- Bounds are **[inclusive, exclusive)**: `FROM ('2025-01-01') TO ('2025-02-01')` includes Jan 1 but excludes Feb 1.
- Perfect for **time-series data** (logs, events, transactions, metrics).
- Enables trivially fast data archival / deletion by dropping old partitions.

**When to use:** Your most common WHERE clause filters on a date/timestamp column, and you have a clear data retention policy.

### 3.2 List Partitioning

Divides data by **explicit sets of values** for the partition key.

```sql
CREATE TYPE order_status AS ENUM ('pending', 'processing', 'shipped', 'delivered', 'cancelled');

CREATE TABLE orders (
    id          BIGSERIAL,
    customer_id BIGINT         NOT NULL,
    status      order_status   NOT NULL,
    total       NUMERIC(12,2)  NOT NULL,
    created_at  TIMESTAMPTZ    NOT NULL DEFAULT now(),
    PRIMARY KEY (id, status)
) PARTITION BY LIST (status);

CREATE TABLE orders_active   PARTITION OF orders FOR VALUES IN ('pending', 'processing');
CREATE TABLE orders_shipped  PARTITION OF orders FOR VALUES IN ('shipped');
CREATE TABLE orders_complete PARTITION OF orders FOR VALUES IN ('delivered');
CREATE TABLE orders_cancelled PARTITION OF orders FOR VALUES IN ('cancelled');
```

**Key properties:**
- Each partition holds rows matching **specific discrete values**.
- Excellent when queries almost always filter by the partition key value.
- Can group related values into one partition (e.g., `pending` + `processing` → `orders_active`).

**When to use:** Data naturally segments into discrete categories that are queried in isolation (status, region, tenant).

### 3.3 Hash Partitioning

Distributes rows across partitions using a **hash function** with modulus arithmetic.

```sql
CREATE TABLE sessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     BIGINT NOT NULL,
    data        JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
) PARTITION BY HASH (id);

CREATE TABLE sessions_0 PARTITION OF sessions FOR VALUES WITH (MODULUS 4, REMAINDER 0);
CREATE TABLE sessions_1 PARTITION OF sessions FOR VALUES WITH (MODULUS 4, REMAINDER 1);
CREATE TABLE sessions_2 PARTITION OF sessions FOR VALUES WITH (MODULUS 4, REMAINDER 2);
CREATE TABLE sessions_3 PARTITION OF sessions FOR VALUES WITH (MODULUS 4, REMAINDER 3);
```

**Key properties:**
- Guarantees **even distribution** regardless of data skew.
- The hash function is deterministic — given the same key, always routes to the same partition.
- You **cannot drop individual partitions** to remove a logical subset of data (unlike range/list).
- Adding partitions later requires **re-hashing** (redistributing all data).

**When to use:** No natural range or categorical split exists; you just want to break a massive table into N equally-sized pieces for I/O and VACUUM benefits. This is the rarest strategy.

### 3.4 Comparison Table

| Aspect | Range | List | Hash |
|---|---|---|---|
| Partition key | Continuous range (dates, numbers) | Discrete values (enums, categories) | Any hashable column |
| Data distribution | Based on range boundaries | Based on explicit value mapping | Even (hash-based) |
| Archival/deletion | Drop old partitions ✅ | Drop categorical partitions ✅ | Cannot target logical subsets ❌ |
| Adding partitions | Add new range ✅ | Add new value set ✅ | Requires modulus change + rehash ❌ |
| Partition pruning | WHERE key >= X AND key < Y | WHERE key IN (...) | WHERE key = X |
| Best for | Time-series, sequential data | Status/category-based access | Even distribution needs |

---

## 4. Partitioning and Indexing — The Deep Connection

This is one of the most important and least-understood aspects of partitioning.

### 4.1 Partitioning as an "Index Substitute"

The PostgreSQL docs say it explicitly:

> "Partitioning effectively substitutes for the upper tree levels of indexes, making it more likely that the heavily-used parts of the indexes fit in memory."

What does this mean? Consider a B-tree index on `created_at`:

```
Without partitioning (one massive B-tree):

         [Root Page]                    ← needs to be in RAM
        /     |      \
   [Internal] [Internal] [Internal]    ← needs to be in RAM
   /  |  \    /  |  \    /  |  \
 [Leaf][Leaf][Leaf][Leaf][Leaf][Leaf]   ← ideally in RAM
   ↓    ↓    ↓    ↓    ↓    ↓
 [Heap pages scattered across 500 GB]
```

```
With monthly partitions (12 small B-trees):

 Partition 2025_01:   [Root]           ← easily fits in RAM
                     /      \
                  [Leaf]  [Leaf]
                    ↓       ↓
               [Heap pages ~10 GB]     ← hot month's data

 Partition 2025_02:   [Root]
                     /      \
                  ...
```

The partition boundary itself acts as the "root + upper internal nodes" of the monolithic index. PostgreSQL's **partition pruning** eliminates irrelevant partitions *before* index traversal even begins.

### 4.2 Per-Partition Indexes

Each partition maintains its own **independent** indexes:

```sql
-- Index defined on the parent → automatically created on each partition
CREATE INDEX idx_events_user_id ON events (user_id);
-- This creates:
--   idx_events_user_id           (on parent, virtual)
--   events_2025_01_user_id_idx   (on events_2025_01, physical)
--   events_2025_02_user_id_idx   (on events_2025_02, physical)
--   events_2025_03_user_id_idx   (on events_2025_03, physical)
```

**Implications:**
- Each index is smaller → better cache hit rate.
- Index maintenance (inserts, splits) is cheaper per partition.
- You can have **different indexes on different partitions** if you create them directly on child tables (advanced use case).
- Index rebuilds (`REINDEX`) affect only one partition at a time.

### 4.3 When Indexes Become Less Necessary

For queries that access a **large fraction** of a single partition, a sequential scan of that partition can be faster than index lookups:

```sql
-- Suppose events_2025_01 has 5M rows and the query needs 3M of them
SELECT * FROM events WHERE created_at >= '2025-01-01' AND created_at < '2025-02-01'
  AND event_type = 'purchase';

-- Sequential scan of events_2025_01 (~10 GB) may beat
-- random-access index reads scattered across those pages
```

This is because sequential I/O is **10-100x faster** than random I/O on spinning disks and still significantly faster on SSDs due to read-ahead and caching benefits.

---

## 5. Partitioning and Other Database Subsystems

### 5.1 VACUUM and Autovacuum

VACUUM is PostgreSQL's garbage collector — it reclaims space from dead tuples (rows marked for deletion by MVCC).

**Without partitioning:**
- A single VACUUM on a 500 GB table can take hours.
- It holds lightweight locks that can interfere with other operations.
- If it's interrupted, it starts over from the beginning.
- The `autovacuum_max_workers` setting limits concurrent VACUUM operations.

**With partitioning:**
- Each partition is VACUUMed independently.
- A 10 GB partition takes minutes, not hours.
- Autovacuum can process multiple partitions concurrently (one worker per partition).
- Dropped partitions need no VACUUM at all — the space is reclaimed instantly.

### 5.2 VACUUM and Bulk Deletes — The Killer Feature

This is arguably the **single biggest production benefit** of partitioning:

```sql
-- WITHOUT partitioning: delete old data
DELETE FROM events WHERE created_at < '2024-01-01';
-- → Marks millions of rows as dead
-- → Requires VACUUM to reclaim space
-- → VACUUM can take hours on a large table
-- → Table bloat in the meantime

-- WITH partitioning: drop old partition
ALTER TABLE events DETACH PARTITION events_2023_12;
DROP TABLE events_2023_12;
-- → Instant. No dead tuples. No VACUUM needed. No bloat.
```

### 5.3 Query Planning and Partition Pruning

**Partition pruning** is the mechanism by which PostgreSQL's query planner excludes irrelevant partitions from a query's execution plan.

There are two phases:
1. **Plan-time pruning** — partitions are eliminated during query planning based on constant values in WHERE clauses.
2. **Execution-time pruning** (PostgreSQL 11+) — partitions are eliminated at runtime based on parameter values (e.g., prepared statements, subquery results).

```sql
-- Pruning works here (constant value):
EXPLAIN SELECT * FROM events WHERE created_at = '2025-02-15';
-- Plan shows: Scan on events_2025_02 only

-- Pruning also works here (execution-time, PG 11+):
PREPARE q AS SELECT * FROM events WHERE created_at = $1;
EXECUTE q('2025-02-15');
-- Plan prunes at execution time

-- Pruning DOES NOT work here (expression on partition key):
SELECT * FROM events WHERE EXTRACT(YEAR FROM created_at) = 2025;
-- Scans ALL partitions — the planner can't infer the range
```

**Critical setting:**
```sql
-- Must be ON (default since PG 11)
SET enable_partition_pruning = on;
```

### 5.4 Constraints and Foreign Keys

**Unique constraints / Primary keys:**
- **Must include the partition key.** This is a hard PostgreSQL requirement.
- Reason: PostgreSQL can only enforce uniqueness within a single index, and each partition has its own index. Without the partition key in the constraint, PostgreSQL cannot guarantee cross-partition uniqueness.

```sql
-- ✅ Works: partition key (created_at) is part of the PK
PRIMARY KEY (id, created_at)

-- ❌ Fails: cannot create unique constraint without partition key
PRIMARY KEY (id)
-- ERROR: unique constraint on partitioned table must include all partitioning columns
```

**Foreign keys:**
- A partitioned table **can reference** (point to) a non-partitioned table via FK. ✅
- A non-partitioned table **can reference** a partitioned table (PostgreSQL 12+). ✅
- Foreign keys between two partitioned tables work but require careful testing.
- **Performance note**: FK constraint checks on INSERT/UPDATE touch the referenced table's index. If the referenced table is also partitioned, this adds routing overhead.

### 5.5 Sub-Partitioning

Partitions can themselves be partitioned, creating a hierarchy:

```sql
CREATE TABLE events (
    id BIGSERIAL,
    region TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (id, created_at, region)
) PARTITION BY RANGE (created_at);

CREATE TABLE events_2025_01 PARTITION OF events
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01')
    PARTITION BY LIST (region);

CREATE TABLE events_2025_01_us PARTITION OF events_2025_01
    FOR VALUES IN ('us-east', 'us-west');

CREATE TABLE events_2025_01_eu PARTITION OF events_2025_01
    FOR VALUES IN ('eu-west', 'eu-central');
```

**Use sparingly** — sub-partitioning multiplies the number of physical tables and can significantly increase query planning overhead.

### 5.6 TOAST (The Oversized-Attribute Storage Technique)

Each partition has its own TOAST table. This means large JSONB/TEXT values stored via TOAST are co-located with their partition, improving locality for partition-scoped queries.

---

## 6. Declarative Partitioning Syntax (PostgreSQL 10+)

Before PostgreSQL 10, partitioning required manual inheritance + triggers (messy and error-prone). Since PG 10, **declarative partitioning** is the standard:

### 6.1 Creating a Partitioned Table

```sql
CREATE TABLE measurements (
    id          BIGSERIAL,
    sensor_id   INT          NOT NULL,
    value       DOUBLE PRECISION NOT NULL,
    recorded_at TIMESTAMPTZ  NOT NULL DEFAULT now(),

    -- Partition key MUST be in PK/UNIQUE constraints
    PRIMARY KEY (id, recorded_at)
) PARTITION BY RANGE (recorded_at);
```

### 6.2 Creating Partitions

```sql
-- Explicit partition for a month
CREATE TABLE measurements_2025_01 PARTITION OF measurements
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

-- Default partition (catches everything else)
CREATE TABLE measurements_default PARTITION OF measurements DEFAULT;
```

### 6.3 Attaching and Detaching Partitions

```sql
-- Detach a partition (makes it a standalone table)
ALTER TABLE measurements DETACH PARTITION measurements_2024_01;

-- Detach concurrently (PG 14+) — doesn't block reads/writes on other partitions
ALTER TABLE measurements DETACH PARTITION measurements_2024_01 CONCURRENTLY;

-- Re-attach an existing table as a partition
ALTER TABLE measurements ATTACH PARTITION measurements_2025_04
    FOR VALUES FROM ('2025-04-01') TO ('2025-05-01');
-- NOTE: ATTACH validates that ALL rows in the table satisfy the partition constraint.
-- For large tables, this can be slow. Pre-add a CHECK constraint to speed it up:

ALTER TABLE measurements_2025_04
    ADD CONSTRAINT partition_check
    CHECK (recorded_at >= '2025-04-01' AND recorded_at < '2025-05-01');
-- Now ATTACH will skip the validation scan (constraint already proves bounds)
```

### 6.4 Inserting Data

```sql
-- Inserts are routed automatically — no application changes needed
INSERT INTO measurements (sensor_id, value, recorded_at)
VALUES (42, 23.5, '2025-02-15 10:30:00+00');
-- → Automatically inserted into measurements_2025_02
```

### 6.5 Indexes on Partitioned Tables

```sql
-- Create index on parent → automatically propagated to all existing and future partitions
CREATE INDEX idx_measurements_sensor ON measurements (sensor_id);

-- Create index only on a specific partition (not propagated)
CREATE INDEX idx_measurements_2025_01_value ON measurements_2025_01 (value);
```

---

## 7. Real-World Production Implementation

### 7.1 Step-by-Step: Partitioning a New Table

**Scenario:** You're building an event tracking system expecting 100M+ events/month.

#### Step 1: Design the partition key

```
Question: What column appears in nearly every WHERE clause?
Answer:   created_at (time-based queries dominate)

Question: What's the right granularity?
Answer:   Monthly — balances partition size (~10-50 GB) with management overhead
```

#### Step 2: Create the partitioned table

```sql
CREATE TABLE events (
    id          BIGINT GENERATED ALWAYS AS IDENTITY,
    user_id     BIGINT       NOT NULL,
    event_type  TEXT         NOT NULL,
    properties  JSONB        DEFAULT '{}',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),

    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
```

#### Step 3: Create initial partitions (current + future buffer)

```sql
-- Create partitions for current month + 3 months ahead
CREATE TABLE events_2025_02 PARTITION OF events
    FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');
CREATE TABLE events_2025_03 PARTITION OF events
    FOR VALUES FROM ('2025-03-01') TO ('2025-04-01');
CREATE TABLE events_2025_04 PARTITION OF events
    FOR VALUES FROM ('2025-04-01') TO ('2025-05-01');
CREATE TABLE events_2025_05 PARTITION OF events
    FOR VALUES FROM ('2025-05-01') TO ('2025-06-01');

-- Default partition as safety net
CREATE TABLE events_default PARTITION OF events DEFAULT;
```

#### Step 4: Create indexes

```sql
-- These propagate to all partitions automatically
CREATE INDEX idx_events_user_id    ON events (user_id);
CREATE INDEX idx_events_event_type ON events (event_type);
CREATE INDEX idx_events_created_at ON events (created_at);
```

#### Step 5: Automate partition creation

Use **pg_partman** (the industry-standard extension) or a cron job:

```sql
-- Option A: pg_partman (recommended)
CREATE EXTENSION pg_partman;

SELECT partman.create_parent(
    p_parent_table  := 'public.events',
    p_control       := 'created_at',
    p_type          := 'native',         -- use declarative partitioning
    p_interval      := '1 month',
    p_premake       := 3                 -- create 3 months ahead
);

-- Run maintenance periodically (e.g., daily cron)
SELECT partman.run_maintenance();
```

```sql
-- Option B: Manual cron function
CREATE OR REPLACE FUNCTION create_monthly_partition()
RETURNS void AS $$
DECLARE
    partition_date DATE;
    partition_name TEXT;
    start_date     DATE;
    end_date       DATE;
BEGIN
    -- Create partition for 3 months from now
    partition_date := date_trunc('month', now() + INTERVAL '3 months');
    partition_name := 'events_' || to_char(partition_date, 'YYYY_MM');
    start_date     := partition_date;
    end_date       := partition_date + INTERVAL '1 month';

    -- Check if partition already exists
    IF NOT EXISTS (
        SELECT 1 FROM pg_tables WHERE tablename = partition_name
    ) THEN
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF events FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date
        );
        RAISE NOTICE 'Created partition: %', partition_name;
    END IF;
END;
$$ LANGUAGE plpgsql;
```

#### Step 6: Implement retention policy

```sql
-- Archive and drop partitions older than 12 months
-- Run monthly via cron or pg_partman

-- Step 1: Detach (stops new writes to this partition)
ALTER TABLE events DETACH PARTITION events_2024_01 CONCURRENTLY;

-- Step 2: Optionally dump for archival
-- pg_dump -t events_2024_01 mydb > events_2024_01_archive.sql

-- Step 3: Drop
DROP TABLE events_2024_01;
```

### 7.2 Migrating an Existing Large Table to Partitioned

This is the hard part. PostgreSQL does **not** support `ALTER TABLE ... PARTITION BY` on existing tables. You must create a new partitioned table and migrate data.

#### Strategy A: Offline migration (simpler, requires downtime)

```sql
-- 1. Create new partitioned table
CREATE TABLE events_new ( ... ) PARTITION BY RANGE (created_at);
-- Create all needed partitions...

-- 2. Copy data in batches
INSERT INTO events_new
SELECT * FROM events_old
WHERE created_at >= '2025-01-01' AND created_at < '2025-02-01';
-- Repeat for each month...

-- 3. Swap tables (during maintenance window)
BEGIN;
ALTER TABLE events RENAME TO events_old;
ALTER TABLE events_new RENAME TO events;
COMMIT;

-- 4. Update sequences, foreign keys, views, etc.
-- 5. Drop old table after verification period
```

#### Strategy B: Online migration (zero-downtime, more complex)

```
1. Create the new partitioned table alongside the old one.
2. Set up a trigger on the old table to dual-write INSERTs to the new table.
3. Backfill historical data from old → new in batches (using date ranges).
4. Once backfill is complete and the trigger has kept the new table current:
   a. Verify row counts match.
   b. Switch application reads to the new table.
   c. Switch application writes to the new table (remove trigger).
   d. Drop the old table after a verification period.
```

Alternatively, use **logical replication** (PG 10+) to stream changes from the old table to the new partitioned table, then cut over.

---

## 8. Production Gotchas and Pitfalls

### 8.1 The Partition Key Must Be in Every Unique Constraint

```sql
-- ❌ This WILL fail
CREATE TABLE events (
    id BIGINT PRIMARY KEY,                    -- no partition key
    created_at TIMESTAMPTZ NOT NULL
) PARTITION BY RANGE (created_at);

-- ERROR: unique constraint on partitioned table must include all partitioning columns

-- ✅ This works
CREATE TABLE events (
    id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (id, created_at)              -- partition key included
) PARTITION BY RANGE (created_at);
```

**Impact:** If your application relies on `id` alone being unique, you'll need to enforce this at the application level or use a UUID that's inherently unique.

### 8.2 Queries Without the Partition Key Scan ALL Partitions

```sql
-- ❌ No partition pruning — scans every partition
SELECT * FROM events WHERE user_id = 12345;

-- ✅ Partition pruning kicks in
SELECT * FROM events WHERE user_id = 12345 AND created_at >= '2025-02-01' AND created_at < '2025-03-01';
```

**Impact:** Every query that touches the partitioned table should ideally include a filter on the partition key. **Communicate this to your entire team.**

### 8.3 Expression-Based Filters Defeat Pruning

```sql
-- ❌ Pruning CANNOT work (expression wraps the partition key)
SELECT * FROM events WHERE EXTRACT(YEAR FROM created_at) = 2025;
SELECT * FROM events WHERE created_at::date = '2025-02-15';
SELECT * FROM events WHERE date_trunc('month', created_at) = '2025-02-01';

-- ✅ Rewrite as range comparison
SELECT * FROM events WHERE created_at >= '2025-01-01' AND created_at < '2026-01-01';
SELECT * FROM events WHERE created_at >= '2025-02-15' AND created_at < '2025-02-16';
SELECT * FROM events WHERE created_at >= '2025-02-01' AND created_at < '2025-03-01';
```

**Rule:** The partition key column must appear **bare** (not wrapped in a function) on one side of the comparison operator.

### 8.4 UPDATE on Partition Key Causes Row Movement

```sql
-- This moves the row from events_2025_02 to events_2025_03
UPDATE events SET created_at = '2025-03-15' WHERE id = 42 AND created_at = '2025-02-15';
```

Under the hood, PostgreSQL performs a **DELETE + INSERT** across partitions. This is:
- Slower than a regular UPDATE
- Generates additional WAL
- Can cause issues with triggers and foreign keys

**Recommendation:** Avoid updating partition key columns if possible. If unavoidable, ensure `enable_partition_pruning = on` and test thoroughly.

### 8.5 Too Many Partitions = Planning Overhead

The query planner evaluates each partition's constraints during planning. With many partitions:

| Partition Count | Planning Overhead |
|---|---|
| 10-50 | Negligible |
| 50-200 | Measurable but acceptable |
| 200-1000 | Can add 10-100ms to planning |
| 1000+ | Significant; consider coarser granularity |

**Recommendation:** Keep partition count reasonable. If you have daily partitions for 10 years, that's 3,650 partitions — consider monthly (120 partitions) or dropping old partitions.

### 8.6 Default Partition Can Silently Accumulate Junk

If you create a DEFAULT partition and forget to create a partition for a new month, all new data silently lands in the default partition. This:
- Defeats partitioning benefits for that data
- Makes it painful to create the correct partition later (must move data out of default first)
- Can grow silently until someone notices

**Recommendation:**
- Monitor default partition size — it should always be near-zero.
- Set up alerts on `pg_total_relation_size('events_default')`.
- Create partitions well ahead of time (3+ periods in advance).

### 8.7 ATTACH PARTITION Validates All Rows

```sql
ALTER TABLE events ATTACH PARTITION events_2025_06
    FOR VALUES FROM ('2025-06-01') TO ('2025-07-01');
```

This scans **every row** in `events_2025_06` to verify it satisfies the partition constraint. For large tables, this blocks for a long time.

**Workaround:** Pre-add a matching CHECK constraint before attaching:

```sql
-- Add the constraint first (can be done with NOT VALID + VALIDATE separately)
ALTER TABLE events_2025_06
    ADD CONSTRAINT chk_events_2025_06
    CHECK (recorded_at >= '2025-06-01' AND recorded_at < '2025-07-01');

-- Now ATTACH skips the validation scan
ALTER TABLE events ATTACH PARTITION events_2025_06
    FOR VALUES FROM ('2025-06-01') TO ('2025-07-01');
```

### 8.8 Foreign Keys Referencing Partitioned Tables

While supported since PostgreSQL 12, FK checks on partitioned tables involve:
1. Determining which partition holds the referenced row.
2. Looking up the row in that partition's index.

This adds overhead on every INSERT/UPDATE that touches the FK. For high-throughput tables, benchmark this carefully.

### 8.9 ORM and Tooling Compatibility

Some ORMs and database tools may not fully understand partitioned tables:
- **Schema introspection** might show partitions as separate tables.
- **Migration tools** might not support `PARTITION BY` syntax natively.
- **pg_dump** handles partitioned tables correctly, but older versions may have quirks.

**Recommendation:** Test your entire toolchain (ORM, migration framework, monitoring, backup) against partitioned tables before going to production.

### 8.10 Cross-Partition Aggregation

```sql
-- This touches ALL partitions, aggregating results
SELECT COUNT(*), AVG(value) FROM events;
```

PostgreSQL can parallelize this across partitions (`Parallel Append` in EXPLAIN), but it's still more expensive than aggregating a single table. If you frequently need full-table aggregations, consider:
- Materialized views refreshed periodically
- Pre-computed summary tables
- Application-level caching

### 8.11 Sequence / IDENTITY Gaps Across Partitions

If using `BIGSERIAL` or `GENERATED ALWAYS AS IDENTITY`, the sequence is shared across all partitions. This works correctly but can lead to non-contiguous IDs within a single partition (which is usually fine, but worth knowing).

---

## 9. Monitoring and Observability

### 9.1 Check Partition Sizes

```sql
-- Size of each partition including indexes and TOAST
SELECT
    c.relname                                           AS partition_name,
    pg_size_pretty(pg_total_relation_size(c.oid))       AS total_size,
    pg_size_pretty(pg_relation_size(c.oid))             AS data_size,
    pg_size_pretty(pg_indexes_size(c.oid))              AS index_size,
    n_live_tup                                          AS live_rows,
    n_dead_tup                                          AS dead_rows,
    last_autovacuum
FROM pg_inherits i
JOIN pg_class c ON c.oid = i.inhrelid
JOIN pg_stat_user_tables s ON s.relid = c.oid
WHERE i.inhparent = 'events'::regclass
ORDER BY c.relname;
```

### 9.2 Verify Partition Pruning in Queries

```sql
-- Always check EXPLAIN output for partitioned queries
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT * FROM events
WHERE created_at >= '2025-02-01' AND created_at < '2025-03-01';

-- Look for:
--   "Subplans Removed: N"        ← partitions pruned at plan time
--   "never executed"              ← partitions pruned at execution time
--   Only the relevant partition's Seq Scan / Index Scan appears
```

### 9.3 Monitor Default Partition

```sql
-- Alert if default partition has any rows
SELECT
    pg_size_pretty(pg_total_relation_size('events_default')) AS default_size,
    (SELECT count(*) FROM events_default) AS row_count;

-- In production: set up an alert if row_count > 0
```

### 9.4 Monitor Autovacuum Progress Per Partition

```sql
SELECT
    schemaname,
    relname,
    last_vacuum,
    last_autovacuum,
    vacuum_count,
    autovacuum_count,
    n_dead_tup,
    n_live_tup,
    CASE WHEN n_live_tup > 0
         THEN round(100.0 * n_dead_tup / n_live_tup, 2)
         ELSE 0
    END AS dead_pct
FROM pg_stat_user_tables
WHERE relname LIKE 'events_%'
ORDER BY n_dead_tup DESC;
```

### 9.5 Check for Missing Partitions

```sql
-- For monthly range partitions, verify no gaps exist
WITH months AS (
    SELECT generate_series(
        '2025-01-01'::date,
        date_trunc('month', now()) + INTERVAL '3 months',
        '1 month'
    )::date AS month_start
)
SELECT
    m.month_start,
    'events_' || to_char(m.month_start, 'YYYY_MM') AS expected_partition,
    EXISTS (
        SELECT 1 FROM pg_tables
        WHERE tablename = 'events_' || to_char(m.month_start, 'YYYY_MM')
    ) AS exists
FROM months m
ORDER BY m.month_start;
```

---

## 10. When NOT to Partition

Partitioning is not always the answer. **Avoid it when:**

| Condition | Why |
|---|---|
| **Table fits in RAM** (< a few GB) | Indexes already work efficiently; partitioning adds overhead without benefit |
| **No natural partition key** | Forced partitioning leads to poor pruning and cross-partition queries |
| **Queries always span all data** | `SELECT COUNT(*) FROM t` hits every partition — no pruning benefit |
| **High-frequency UPDATEs on partition key** | Row movement between partitions is expensive |
| **Team lacks operational maturity** | Partition management (creation, archival, monitoring) is an ongoing burden |
| **Low query volume** | If the table is queried infrequently, the optimization overhead isn't justified |

**Bottom line:** Partition when you have a clear, measurable problem with table size, query performance, or data lifecycle management — not speculatively.

---

## 11. Quick-Reference Cheat Sheet

### Create a Range-Partitioned Table
```sql
CREATE TABLE t (..., PRIMARY KEY (id, partition_col))
PARTITION BY RANGE (partition_col);

CREATE TABLE t_2025_01 PARTITION OF t
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

CREATE TABLE t_default PARTITION OF t DEFAULT;
```

### Verify Pruning
```sql
EXPLAIN (ANALYZE) SELECT * FROM t WHERE partition_col = '2025-01-15';
-- Look for: "Subplans Removed" or only relevant partition in plan
```

### Archive Old Data
```sql
ALTER TABLE t DETACH PARTITION t_2024_01 CONCURRENTLY;  -- PG 14+
DROP TABLE t_2024_01;
```

### Monitor Partition Health
```sql
SELECT relname, pg_size_pretty(pg_total_relation_size(oid))
FROM pg_class WHERE relname LIKE 't_%' AND relkind = 'r'
ORDER BY pg_total_relation_size(oid) DESC;
```

### Key Settings
```sql
SET enable_partition_pruning = on;       -- default: on (PG 11+)
SET constraint_exclusion = partition;    -- default: partition
```

---

## References

1. [dyl.dog — Everything You Need to Know About PostgreSQL Partitioning](https://dyl.dog/everything-you-need-to-know-about-postgres-partitioning/)
2. [PostgreSQL 16 Documentation — Table Partitioning](https://www.postgresql.org/docs/16/ddl-partitioning.html)
