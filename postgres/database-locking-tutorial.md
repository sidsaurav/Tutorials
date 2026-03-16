# 🔐 Database Locking Mastery — A PostgreSQL Deep Dive

> A comprehensive, self-sufficient tutorial for senior backend engineers.
> Covers everything from concurrency anomalies through MVCC internals, deadlocks,
> real-world patterns, and production monitoring — with a strong PostgreSQL focus.

---

## Table of Contents

1. [Why Locks Exist — The Concurrency Problem](#chapter-1-why-locks-exist--the-concurrency-problem)
2. [Lock Types in PostgreSQL — A Deep Taxonomy](#chapter-2-lock-types-in-postgresql--a-deep-taxonomy)
3. [Lock Granularity in PostgreSQL](#chapter-3-lock-granularity-in-postgresql)
4. [Pessimistic vs. Optimistic Locking — A Deep Comparison](#chapter-4-pessimistic-vs-optimistic-locking--a-deep-comparison)
5. [Isolation Levels — PostgreSQL Deep Dive](#chapter-5-isolation-levels--postgresql-deep-dive)
6. [MVCC — PostgreSQL's Concurrency Engine](#chapter-6-mvcc--postgresqls-concurrency-engine)
7. [Deadlocks in PostgreSQL](#chapter-7-deadlocks-in-postgresql)
8. [Real-World Locking Patterns in PostgreSQL](#chapter-8-real-world-locking-patterns-in-postgresql)
9. [Monitoring, Debugging & Performance Tuning](#chapter-9-monitoring-debugging--performance-tuning)
10. [PostgreSQL vs. MySQL/InnoDB — Lock Comparison & Quick Reference](#chapter-10-postgresql-vs-mysqlinnodb--lock-comparison--quick-reference)

---


# Chapter 1: Why Locks Exist — The Concurrency Problem

## The Fundamental Challenge

Every production database serves dozens to thousands of concurrent connections. Each connection may be reading and writing overlapping sets of rows at the same time. Without a coordination mechanism, these concurrent operations produce **data anomalies** — situations where the database state is logically incorrect even though every individual statement executed correctly.

Locks are the database's answer to this coordination problem. They force concurrent transactions to **wait** when their operations would conflict, serializing access to shared data and preserving correctness.

## The Five Concurrency Anomalies

The SQL standard and database literature define five key anomalies that arise from unrestricted concurrency. Understanding each one is essential because different **isolation levels** protect against different subsets.

### 1. Dirty Read

A transaction reads data written by another transaction **that has not yet committed**.

```
Timeline:
─────────────────────────────────────────────────────────────
T1: BEGIN
T1: UPDATE accounts SET balance = 0 WHERE id = 1;   -- balance was 1000
                                                      -- T1 has NOT committed yet
T2: BEGIN
T2: SELECT balance FROM accounts WHERE id = 1;       -- reads 0 (DIRTY!)
T2: -- Makes business decision based on balance = 0

T1: ROLLBACK;                                         -- balance reverts to 1000
                                                      -- T2's decision was based on
                                                      -- data that NEVER existed
─────────────────────────────────────────────────────────────
```

**PostgreSQL behavior**: Dirty reads are **impossible** in PostgreSQL, even at `READ UNCOMMITTED`. PostgreSQL's MVCC architecture means uncommitted changes are simply invisible to other transactions. The `READ UNCOMMITTED` level in PostgreSQL is silently treated as `READ COMMITTED`.

```sql
-- Prove it: PostgreSQL prevents dirty reads even at READ UNCOMMITTED
-- Session 1:
BEGIN;
SET TRANSACTION ISOLATION LEVEL READ UNCOMMITTED;
UPDATE accounts SET balance = 0 WHERE id = 1;
-- Don't commit yet

-- Session 2:
BEGIN;
SET TRANSACTION ISOLATION LEVEL READ UNCOMMITTED;
SELECT balance FROM accounts WHERE id = 1;
-- Returns 1000, NOT 0. PostgreSQL ignores READ UNCOMMITTED.
```

### 2. Non-Repeatable Read

A transaction reads the same row twice and gets **different values** because another transaction modified and committed the row in between.

```
Timeline:
─────────────────────────────────────────────────────────────
T1: BEGIN
T1: SELECT balance FROM accounts WHERE id = 1;       -- returns 1000

T2: BEGIN
T2: UPDATE accounts SET balance = 500 WHERE id = 1;
T2: COMMIT;                                           -- committed!

T1: SELECT balance FROM accounts WHERE id = 1;       -- returns 500
                                                      -- Different from first read!
T1: COMMIT
─────────────────────────────────────────────────────────────
```

**PostgreSQL behavior at READ COMMITTED (default)**: This anomaly **is allowed**. Each `SELECT` statement sees a fresh snapshot of committed data. If another transaction commits between your two `SELECT`s, you'll see the new value.

**PostgreSQL behavior at REPEATABLE READ**: This anomaly is **prevented**. The transaction snapshot is taken at the first statement, and all subsequent reads see that same snapshot regardless of other commits.

```sql
-- Demonstrate non-repeatable read at READ COMMITTED
-- Session 1:
BEGIN;
SELECT balance FROM accounts WHERE id = 1;  -- sees 1000

-- Session 2 (execute while Session 1 is open):
UPDATE accounts SET balance = 500 WHERE id = 1;
-- auto-commits (or explicit COMMIT)

-- Back to Session 1:
SELECT balance FROM accounts WHERE id = 1;  -- sees 500 (different!)
COMMIT;

-- Now prevent it with REPEATABLE READ:
-- Session 1:
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ;
SELECT balance FROM accounts WHERE id = 1;  -- sees 1000

-- Session 2:
UPDATE accounts SET balance = 500 WHERE id = 1;

-- Session 1:
SELECT balance FROM accounts WHERE id = 1;  -- STILL sees 1000!
COMMIT;
```

### 3. Phantom Read

A transaction re-executes a **range query** and finds new rows that were inserted (and committed) by another transaction.

```
Timeline:
─────────────────────────────────────────────────────────────
T1: BEGIN
T1: SELECT * FROM orders WHERE amount > 100;          -- returns 5 rows

T2: BEGIN
T2: INSERT INTO orders (amount) VALUES (200);
T2: COMMIT;

T1: SELECT * FROM orders WHERE amount > 100;          -- returns 6 rows
                                                       -- A "phantom" row appeared!
T1: COMMIT
─────────────────────────────────────────────────────────────
```

**PostgreSQL at REPEATABLE READ**: Phantom reads are **prevented entirely**. Because the transaction uses a frozen snapshot of the database, it cannot see rows inserted by other transactions that committed after the snapshot was established. Even **locking reads** (`SELECT FOR UPDATE`) only find and lock rows mapped in the original snapshot, preventing phantom rows completely.

### 4. Lost Update

Two transactions read the same row, compute a new value based on what they read, and write back. The second write **overwrites** the first, and the first transaction's changes vanish silently.

```
Timeline (balance starts at 1000):
─────────────────────────────────────────────────────────────
T1: BEGIN
T1: SELECT balance FROM accounts WHERE id = 1;       -- reads 1000
T2: BEGIN
T2: SELECT balance FROM accounts WHERE id = 1;       -- reads 1000

T1: UPDATE accounts SET balance = 1000 + 200 WHERE id = 1;  -- sets to 1200
T1: COMMIT

T2: UPDATE accounts SET balance = 1000 - 300 WHERE id = 1;  -- sets to 700
T2: COMMIT
                                                      -- Final: 700
                                                      -- Expected: 1000 + 200 - 300 = 900
                                                      -- T1's +200 is LOST
─────────────────────────────────────────────────────────────
```

**Prevention in PostgreSQL:**

```sql
-- Method 1: Atomic UPDATE (no read-modify-write cycle)
UPDATE accounts SET balance = balance + 200 WHERE id = 1;
-- This is safe because PostgreSQL locks the row during UPDATE,
-- and the 'balance' reference reads the CURRENT locked value.

-- Method 2: SELECT FOR UPDATE (pessimistic lock)
BEGIN;
SELECT balance FROM accounts WHERE id = 1 FOR UPDATE;
-- Row is now exclusively locked — T2's SELECT FOR UPDATE will BLOCK
UPDATE accounts SET balance = 1200 WHERE id = 1;
COMMIT;

-- Method 3: REPEATABLE READ + retry
-- At REPEATABLE READ, T2's UPDATE would detect the conflict
-- and raise: ERROR: could not serialize access due to concurrent update
```

### 5. Write Skew

Two transactions read overlapping data, make decisions based on what they read, and write to **different** rows. Each transaction's write is individually correct, but the combined result violates a constraint.

```
Timeline:
─────────────────────────────────────────────────────────────
-- Constraint: At least one doctor must be on call.
-- Currently: Alice (on_call=true), Bob (on_call=true)

T1: BEGIN
T1: SELECT count(*) FROM doctors WHERE on_call = true;  -- returns 2
                                                         -- "Safe to go off call"
T2: BEGIN
T2: SELECT count(*) FROM doctors WHERE on_call = true;  -- returns 2
                                                         -- "Safe to go off call"

T1: UPDATE doctors SET on_call = false WHERE name = 'Alice';
T1: COMMIT

T2: UPDATE doctors SET on_call = false WHERE name = 'Bob';
T2: COMMIT
                                                         -- Result: 0 doctors on call!
                                                         -- Constraint violated!
─────────────────────────────────────────────────────────────
```

**PostgreSQL prevention**: Only the **SERIALIZABLE** isolation level prevents write skew. At SERIALIZABLE, PostgreSQL's SSI (Serializable Snapshot Isolation) would detect the read-write dependency and abort one of the transactions:

```sql
BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE;
SELECT count(*) FROM doctors WHERE on_call = true;
-- ... if another serializable txn also reads and writes ...
UPDATE doctors SET on_call = false WHERE name = 'Alice';
COMMIT;
-- ERROR: could not serialize access due to read/write dependencies
-- among transactions
-- HINT: The transaction might succeed if retried.
```

## How ACID Maps to Locks

| Property      | Mechanism                          | How Locks Help                              |
|---------------|------------------------------------|--------------------------------------------|
| **Atomicity** | WAL (Write-Ahead Log), undo       | Locks protect undo log consistency          |
| **Consistency** | Constraints, triggers            | Locks ensure constraint checks see stable data |
| **Isolation** | **Locks + MVCC**                   | **Primary mechanism** — controls visibility |
| **Durability** | WAL, fsync, checkpoints           | Not directly related to locks               |

## PostgreSQL's Approach: MVCC + Locks

PostgreSQL uses a **hybrid** approach. It doesn't rely purely on locks (which would make readers block writers). Instead:

1. **MVCC** (Multi-Version Concurrency Control) handles read-write isolation — readers never block writers, writers never block readers
2. **Row-level locks** handle write-write conflicts — only one writer can modify a specific row at a time
3. **Predicate tracking** (SSI) handles serialization anomalies at the SERIALIZABLE level

This is fundamentally different from databases like SQL Server that use a **lock-based** model where readers acquire shared locks that block writers (unless you enable RCSI — Read Committed Snapshot Isolation).

> **Key takeaway**: In PostgreSQL, the word "lock" most often refers to **write locks** (row-level exclusive locks for UPDATE/DELETE) and **explicit locks** (FOR UPDATE, FOR SHARE, LOCK TABLE). Regular `SELECT` statements acquire no row locks — they use MVCC snapshots instead.



# Chapter 2: Lock Types in PostgreSQL — A Deep Taxonomy

## Overview

PostgreSQL has a **rich lock system** with multiple levels. Unlike simpler databases, PostgreSQL distinguishes between table-level lock modes, row-level locks, page-level locks, advisory locks, and more. Every lock is tracked in the `pg_locks` system view.

## Table-Level Lock Modes

PostgreSQL defines **eight** table-level lock modes, from weakest to strongest. Understanding them is critical for DDL operations, bulk loads, and avoiding unexpected blocking.

### The Full Compatibility Matrix

| Lock Mode               | AS  | RS  | RE  | SUE | S   | SRE | E   | AE  |
|--------------------------|-----|-----|-----|-----|-----|-----|-----|-----|
| ACCESS SHARE (AS)        | ✅  | ✅  | ✅  | ✅  | ✅  | ✅  | ✅  | ❌  |
| ROW SHARE (RS)           | ✅  | ✅  | ✅  | ✅  | ✅  | ✅  | ❌  | ❌  |
| ROW EXCLUSIVE (RE)       | ✅  | ✅  | ✅  | ✅  | ❌  | ❌  | ❌  | ❌  |
| SHARE UPDATE EXCL (SUE)  | ✅  | ✅  | ✅  | ❌  | ❌  | ❌  | ❌  | ❌  |
| SHARE (S)                | ✅  | ✅  | ❌  | ❌  | ✅  | ❌  | ❌  | ❌  |
| SHARE ROW EXCL (SRE)     | ✅  | ✅  | ❌  | ❌  | ❌  | ❌  | ❌  | ❌  |
| EXCLUSIVE (E)            | ✅  | ❌  | ❌  | ❌  | ❌  | ❌  | ❌  | ❌  |
| ACCESS EXCLUSIVE (AE)    | ❌  | ❌  | ❌  | ❌  | ❌  | ❌  | ❌  | ❌  |

*(✅ = compatible, ❌ = conflicts)*

### Which Operations Acquire Which Lock?

```
ACCESS SHARE
├── SELECT (plain reads)
│
ROW SHARE
├── SELECT FOR UPDATE
├── SELECT FOR NO KEY UPDATE
├── SELECT FOR SHARE
├── SELECT FOR KEY SHARE
│
ROW EXCLUSIVE
├── INSERT
├── UPDATE
├── DELETE
│
SHARE UPDATE EXCLUSIVE
├── VACUUM (not FULL)
├── ANALYZE
├── CREATE INDEX CONCURRENTLY
├── CREATE STATISTICS
├── COMMENT ON
├── REINDEX CONCURRENTLY
│
SHARE
├── CREATE INDEX (without CONCURRENTLY)
│
SHARE ROW EXCLUSIVE
├── CREATE TRIGGER
├── ALTER TABLE (some variants)
│
EXCLUSIVE
├── REFRESH MATERIALIZED VIEW CONCURRENTLY
│
ACCESS EXCLUSIVE
├── DROP TABLE
├── ALTER TABLE (most variants)
├── TRUNCATE
├── REINDEX
├── CLUSTER
├── VACUUM FULL
├── REFRESH MATERIALIZED VIEW (without CONCURRENTLY)
├── LOCK TABLE (default mode)
```

### Why This Matters in Production

```sql
-- DANGER: This blocks ALL reads AND writes on the table
ALTER TABLE orders ADD COLUMN tracking_number TEXT;
-- Acquires ACCESS EXCLUSIVE lock
-- Even a simple SELECT is blocked until this completes!

-- SAFE alternative for large tables (PostgreSQL 11+):
ALTER TABLE orders ADD COLUMN tracking_number TEXT DEFAULT NULL;
-- Adding a column with DEFAULT NULL is instant (no table rewrite)
-- Still takes ACCESS EXCLUSIVE, but for milliseconds instead of hours

-- But beware: this DOES rewrite the table (PostgreSQL < 11, or with NOT NULL)
ALTER TABLE orders ADD COLUMN tracking_number TEXT NOT NULL DEFAULT 'N/A';
-- PostgreSQL 11+ makes this instant too, but older versions rewrite

-- SAFE index creation:
CREATE INDEX CONCURRENTLY idx_orders_status ON orders(status);
-- Acquires SHARE UPDATE EXCLUSIVE — doesn't block reads OR writes
-- Takes longer but doesn't cause downtime
```

## Row-Level Locks

PostgreSQL has four row-level lock modes, each acquired by different SQL patterns:

### The Four Row-Level Lock Modes

| Row Lock Mode           | Acquired By                        | Blocks                    |
|------------------------|------------------------------------|---------------------------|
| **FOR KEY SHARE**       | `SELECT FOR KEY SHARE`            | FOR UPDATE only           |
| **FOR SHARE**           | `SELECT FOR SHARE`                | FOR UPDATE, FOR NO KEY UPDATE |
| **FOR NO KEY UPDATE**   | `UPDATE` (not touching key cols)  | FOR UPDATE, FOR SHARE, FOR NO KEY UPDATE |
| **FOR UPDATE**          | `SELECT FOR UPDATE`, `UPDATE` (key cols), `DELETE` | All other row locks |

### FOR KEY SHARE — The Lightweight Lock

`FOR KEY SHARE` is the weakest row lock. It only prevents the row from being **deleted** or having its **key columns** updated. This is what PostgreSQL automatically acquires for foreign key checks.

```sql
-- parent table: departments (id, name)
-- child table: employees (id, name, department_id REFERENCES departments(id))

-- When you INSERT into employees:
INSERT INTO employees (name, department_id) VALUES ('Alice', 5);
-- PostgreSQL automatically acquires FOR KEY SHARE on departments row id=5
-- This prevents another transaction from DELETING department 5
-- But it does NOT block updating department 5's name!

-- This is why FOR KEY SHARE exists: foreign key checks
-- don't need to block non-key updates.
```

### FOR UPDATE vs FOR NO KEY UPDATE

```sql
-- FOR UPDATE: strongest row lock, blocks everything
BEGIN;
SELECT * FROM accounts WHERE id = 1 FOR UPDATE;
-- Blocks: other FOR UPDATE, FOR NO KEY UPDATE, FOR SHARE, FOR KEY SHARE, UPDATE, DELETE

-- FOR NO KEY UPDATE: automatic for most UPDATEs
BEGIN;
UPDATE accounts SET balance = 500 WHERE id = 1;
-- PostgreSQL uses FOR NO KEY UPDATE if the update does NOT touch
-- columns that are part of a UNIQUE constraint or PRIMARY KEY
-- This allows concurrent FK checks (FOR KEY SHARE) to proceed!

-- FOR UPDATE: automatic for UPDATEs that touch key columns
BEGIN;
UPDATE accounts SET id = 999 WHERE id = 1;
-- Now it's FOR UPDATE because we're modifying the PK
```

> **Key insight**: PostgreSQL distinguishes between "key-updating" and "non-key-updating" operations. This distinction allows **foreign key checks to proceed concurrently** with non-key updates — a significant concurrency improvement over databases that use a single exclusive lock for all writes.

### Row Lock Implementation: Tuple Header Flags

Unlike table-level locks, PostgreSQL row-level locks are **not stored in the lock manager's shared memory**. Instead, they're encoded in the **tuple header** on disk:

```
Tuple Header (PostgreSQL):
┌──────────────────────────────────────────────────────────────┐
│  t_xmin    │  Creating transaction ID                        │
│  t_xmax    │  Deleting/locking transaction ID                │
│  t_infomask│  Flags: HEAP_XMAX_LOCK_ONLY, HEAP_XMAX_KEYSHR_LOCK, etc. │
│  t_infomask2│ More flags: HEAP_KEYS_UPDATED                  │
│  t_ctid    │  Current tuple ID (self-pointer or chain link)  │
└──────────────────────────────────────────────────────────────┘
```

When a transaction acquires a row lock (e.g., `SELECT FOR UPDATE`):
1. `t_xmax` is set to the locking transaction's XID
2. `t_infomask` has `HEAP_XMAX_LOCK_ONLY` set (to distinguish from a real delete)
3. The row is NOT duplicated — the lock is stored in-place

When **multiple** transactions lock the same row (e.g., `FOR SHARE`), PostgreSQL uses a **MultiXact** — a special structure that records a set of transaction IDs and their lock modes:

```sql
-- T1:
SELECT * FROM orders WHERE id = 1 FOR SHARE;
-- t_xmax = T1's XID, infomask = LOCK_ONLY | SHARED

-- T2 (while T1 still holds the lock):
SELECT * FROM orders WHERE id = 1 FOR SHARE;
-- t_xmax = multixact_id, infomask = LOCK_ONLY | MULTI_XACT | SHARED
-- The multixact maps to: {T1: SHARE, T2: SHARE}
```

> This design means **row locks scale to millions of rows** without exhausting shared memory. The trade-off is that row lock information requires reading the heap tuple, which is why lock monitoring queries join `pg_locks` with heap inspection.

## Page-Level Locks

PostgreSQL briefly acquires **page-level share/exclusive locks** during index operations. These are internal, short-lived, and not directly controllable by users. They exist to protect the physical structure of B-tree index pages during splits and merges.

## Spinlocks and Lightweight Locks (LWLocks)

These are **internal** locks used by PostgreSQL itself:

- **Spinlocks**: Ultra-short locks (<1μs) for protecting shared memory data structures. Busy-wait loops.
- **LWLocks**: Lightweight Locks for longer internal operations. Used to protect buffer pool pages, WAL buffers, shared catalogs. Appear in `pg_stat_activity` as `LWLock` wait events.

```sql
-- Check for LWLock contention:
SELECT wait_event_type, wait_event, count(*)
FROM pg_stat_activity
WHERE wait_event_type = 'LWLock'
GROUP BY 1, 2
ORDER BY 3 DESC;

-- Common LWLock waits and what they mean:
-- WALWriteLock       → WAL write bottleneck, tune wal_buffers
-- buffer_content     → Buffer pool contention, check shared_buffers
-- lock_manager       → Lock table contention, too many fine-grained locks
-- XactSLRU           → Transaction status cache contention
```



# Chapter 3: Lock Granularity in PostgreSQL

## The Granularity Hierarchy

Lock granularity refers to the **size of the resource** being locked. PostgreSQL primarily uses row-level and table-level locks, unlike SQL Server which also has page-level locking and lock escalation.

```
┌─────────────────────────────────────────────────────┐
│              Granularity Spectrum                    │
│                                                     │
│  Database ←──── Coarser (less concurrency)          │
│     │                                               │
│  Schema (namespace)                                 │
│     │                                               │
│  Table (relation)     ← LOCK TABLE, DDL             │
│     │                                               │
│  Page (8KB block)     ← Internal only (B-tree ops)  │
│     │                                               │
│  Row (tuple)          ← UPDATE, DELETE, FOR UPDATE  │
│     │                                               │
│  Column               ← NOT supported in PostgreSQL │
│                         (supported in column stores) │
│                                                     │
│               Finer ──── (more concurrency)         │
└─────────────────────────────────────────────────────┘
```

## Row-Level Granularity (PostgreSQL Default for DML)

PostgreSQL always uses row-level locks for DML operations. There is **no lock escalation** — even if you update a million rows, PostgreSQL acquires one million separate row locks. It never "escalates" to a table lock.

```sql
-- This acquires 1,000,000 row-level locks, one per row
-- PostgreSQL does NOT escalate to a table lock
UPDATE large_table SET status = 'archived'
WHERE created_at < '2024-01-01';

-- This is fine because row locks are stored in tuple headers,
-- not in shared memory. The lock manager only tracks:
-- 1. Table-level locks (ACCESS EXCLUSIVE, etc.)
-- 2. Transaction-level row lock WAITS (actual contention)
```

### Why No Lock Escalation?

In SQL Server, if a single transaction acquires more than ~5000 row locks on a table, it escalates to a table lock to save memory. PostgreSQL avoids this by storing row lock info **in the tuple header** (the `t_xmax` field discussed in Chapter 2), not in a shared memory lock table.

**Advantages of no escalation:**
- Predictable behavior — a bulk UPDATE won't suddenly block all readers
- No surprise contention spikes
- Simpler mental model

**Disadvantages:**
- Updating millions of rows creates millions of dead tuples (MVCC overhead)
- VACUUM must clean all those dead tuples
- Large UPDATE/DELETE batches should be chunked for VACUUM friendliness

```sql
-- Best practice: batch large updates
DO $$
DECLARE
  batch_size INT := 10000;
  rows_affected INT;
BEGIN
  LOOP
    UPDATE large_table SET status = 'archived'
    WHERE id IN (
      SELECT id FROM large_table
      WHERE status = 'active' AND created_at < '2024-01-01'
      LIMIT batch_size
      FOR UPDATE SKIP LOCKED  -- avoid blocking concurrent operations
    );
    GET DIAGNOSTICS rows_affected = ROW_COUNT;
    EXIT WHEN rows_affected = 0;
    COMMIT;  -- release locks, allow VACUUM to run
  END LOOP;
END $$;
```

## Table-Level Granularity

Table-level locks are acquired explicitly with `LOCK TABLE` or implicitly by DDL/DML operations.

```sql
-- Explicit table lock (rarely needed in application code)
BEGIN;
LOCK TABLE inventory IN EXCLUSIVE MODE;
-- No other transaction can INSERT, UPDATE, or DELETE
-- SELECTs still work (ACCESS SHARE is compatible with EXCLUSIVE)
UPDATE inventory SET quantity = 0 WHERE warehouse_id = 3;
COMMIT;

-- When would you explicitly lock a table?
-- 1. Constraint validation across rows (e.g., sum of allocations = total)
-- 2. Preventing concurrent modifications during a complex multi-step operation
-- 3. Bulk data loads where consistency of the batch matters
```

### Table Lock Duration: The DDL Problem

The most common table lock problem in production is **DDL blocking everything**:

```
Timeline of an ALTER TABLE disaster:
─────────────────────────────────────────────────────────────────
T0: Long-running SELECT is executing (holds ACCESS SHARE on orders)

T1: ALTER TABLE orders ADD COLUMN foo INT;
    -- Requests ACCESS EXCLUSIVE
    -- Cannot acquire because T0 holds ACCESS SHARE
    -- ALTER TABLE goes into the lock queue and WAITS

T2: New SELECT on orders comes in
    -- Requests ACCESS SHARE
    -- Normally this would succeed (compatible with other ACCESS SHARE)
    -- BUT: the pending ACCESS EXCLUSIVE request is ahead in the queue
    -- So this SELECT ALSO WAITS!

T3: More SELECT queries pile up, ALL blocked
    -- The pending ALTER TABLE acts as a "lock fence"
    -- blocking everything behind it

    Result: Complete table outage because one ALTER TABLE
    is waiting for one long-running query.
─────────────────────────────────────────────────────────────────
```

**Critical mitigation: Always set `lock_timeout` before DDL:**

```sql
-- ALWAYS do this before DDL on production tables:
SET lock_timeout = '3s';  -- fail fast if lock not acquired in 3 seconds

ALTER TABLE orders ADD COLUMN foo INT;
-- If the lock isn't acquired in 3 seconds, this FAILS
-- with ERROR: canceling statement due to lock timeout
-- Then you can retry when the long-running query finishes

-- Reset after:
RESET lock_timeout;
```

## Advisory Locks — Application-Defined Granularity

Advisory locks allow you to lock **arbitrary application-defined resources** identified by a 64-bit integer or a pair of 32-bit integers. The database doesn't enforce any semantics — it's purely a coordination mechanism.

### Session-Level vs. Transaction-Level

```sql
-- SESSION-level advisory lock
-- Held until explicitly released or session disconnects
SELECT pg_advisory_lock(12345);     -- blocking
SELECT pg_try_advisory_lock(12345); -- non-blocking (returns true/false)
-- ... do work ...
SELECT pg_advisory_unlock(12345);

-- TRANSACTION-level advisory lock
-- Auto-released on COMMIT / ROLLBACK
BEGIN;
SELECT pg_advisory_xact_lock(12345);
-- ... do work ...
COMMIT;  -- lock auto-released

-- IMPORTANT: session-level locks are REENTRANT
-- If the same session calls pg_advisory_lock(12345) twice,
-- it must call pg_advisory_unlock(12345) TWICE to release.
-- This is a common bug source.
```

### Practical Advisory Lock Patterns

```sql
-- Pattern 1: Singleton cron job
-- Ensure only one instance of a scheduled job runs
SELECT pg_try_advisory_lock(hashtext('nightly-report-job'));
-- Returns true if we're the only runner, false if another instance is running

-- Pattern 2: Per-entity processing lock (prevents double-processing)
SELECT pg_advisory_xact_lock('orders'::regclass::int, order_id);
-- Uses the table OID + row ID as the lock key
-- Prevents two workers from processing the same order concurrently

-- Pattern 3: Distributed mutex for idempotency
BEGIN;
SELECT pg_advisory_xact_lock(hashtext('process_event_' || event_id::text));
-- Check if already processed
INSERT INTO processed_events (event_id)
VALUES (event_id)
ON CONFLICT DO NOTHING;
-- ... process event ...
COMMIT;

-- Pattern 4: Rate limiting per resource
-- Lock based on resource + time window
SELECT pg_try_advisory_lock(
    hashtext('rate_limit'),
    hashtext(resource_id || '_' || date_trunc('minute', now())::text)
);
```

### Monitoring Advisory Locks

```sql
-- See all advisory locks currently held
SELECT
    l.pid,
    l.classid,     -- first int of the pair (or upper 32 bits of bigint)
    l.objid,       -- second int of the pair (or lower 32 bits of bigint)
    l.granted,
    a.query,
    a.application_name
FROM pg_locks l
JOIN pg_stat_activity a ON l.pid = a.pid
WHERE l.locktype = 'advisory';
```

> **Warning**: Advisory locks are shared across the entire database cluster. If two applications use the same lock ID for different purposes, they'll interfere with each other. Always namespace your lock IDs (e.g., use `hashtext('app_name:purpose:id')`).



# Chapter 4: Pessimistic vs. Optimistic Locking — A Deep Comparison

## The Core Philosophy

These are two **strategies** for handling concurrent modifications — not specific lock types. The choice is one of the most impactful concurrency decisions you'll make as a backend engineer.

| Aspect | Pessimistic | Optimistic |
|--------|-------------|------------|
| **Assumption** | Conflicts are *likely* | Conflicts are *rare* |
| **When locks are acquired** | *Before* reading data | Never (conflict check at write time) |
| **Conflict handling** | Other transactions *wait* | Failed transactions *retry* |
| **Implementation** | Database-level locks | Application-level version checks |
| **Throughput** | Lower under contention | Higher when conflicts rare |
| **Correctness guarantee** | Immediate (lock prevents conflict) | Deferred (detected at write time) |

## Pessimistic Locking in PostgreSQL

### SELECT FOR UPDATE — The Workhorse

```sql
BEGIN;

-- Step 1: Read AND lock the row
SELECT balance FROM accounts WHERE id = 1 FOR UPDATE;
-- This acquires an exclusive row lock on accounts row id=1
-- ANY other transaction that tries FOR UPDATE, FOR SHARE, UPDATE, or DELETE
-- on this row will BLOCK until we COMMIT or ROLLBACK.

-- Step 2: Application logic (safe — we hold the lock)
-- new_balance = balance - 100
-- if new_balance < 0: raise error

-- Step 3: Write
UPDATE accounts SET balance = balance - 100 WHERE id = 1;

COMMIT; -- Row lock released
```

### SELECT FOR UPDATE Variants

PostgreSQL offers four `FOR ...` clause variants, each with different blocking behavior:

```sql
-- FOR UPDATE: Strongest. Blocks everything.
SELECT * FROM orders WHERE id = 1 FOR UPDATE;

-- FOR NO KEY UPDATE: Blocks everything except FOR KEY SHARE.
-- Useful when you're updating non-key columns and don't want to block FK checks.
SELECT * FROM orders WHERE id = 1 FOR NO KEY UPDATE;

-- FOR SHARE: Allows concurrent FOR SHARE, blocks FOR UPDATE/FOR NO KEY UPDATE.
-- Multiple readers can hold this simultaneously.
SELECT * FROM orders WHERE id = 1 FOR SHARE;

-- FOR KEY SHARE: Weakest. Only blocks FOR UPDATE.
-- Used automatically by PostgreSQL for foreign key checks.
SELECT * FROM orders WHERE id = 1 FOR KEY SHARE;
```

### NOWAIT and SKIP LOCKED

```sql
-- NOWAIT: Fail immediately instead of blocking
BEGIN;
SELECT * FROM orders WHERE id = 1 FOR UPDATE NOWAIT;
-- If the row is already locked: ERROR: could not obtain lock on row in relation "orders"
-- Use case: fail-fast in APIs where blocking is worse than a retry

-- SKIP LOCKED: Skip locked rows silently
BEGIN;
SELECT * FROM jobs
WHERE status = 'pending'
ORDER BY created_at
LIMIT 1
FOR UPDATE SKIP LOCKED;
-- Returns the first UNLOCKED pending job
-- If all pending jobs are locked by other workers, returns empty result set
-- Use case: job queue / task distribution (see Chapter 9)
```

### Locking with JOINs — The OF Clause

When selecting from multiple tables, you must specify **which** table's rows to lock:

```sql
-- Lock ONLY the orders rows, not the customers rows
SELECT o.*, c.name
FROM orders o
JOIN customers c ON o.customer_id = c.id
WHERE o.status = 'pending'
FOR UPDATE OF o;  -- critical: lock orders only

-- Without 'OF o', PostgreSQL would lock rows in BOTH tables
-- This is almost never what you want and increases deadlock risk

-- You can lock multiple tables selectively:
SELECT o.*, i.quantity
FROM orders o
JOIN inventory i ON o.product_id = i.product_id
WHERE o.id = 42
FOR UPDATE OF o, i;  -- lock rows in both orders and inventory
```

### Subqueries and FOR UPDATE

```sql
-- WARNING: FOR UPDATE does NOT work with certain query features
-- These will raise errors:

-- ❌ Aggregate functions
SELECT count(*) FROM orders WHERE status = 'pending' FOR UPDATE;
-- ERROR: FOR UPDATE is not allowed with aggregate functions

-- ❌ DISTINCT
SELECT DISTINCT customer_id FROM orders FOR UPDATE;
-- ERROR: FOR UPDATE is not allowed with DISTINCT clause

-- ❌ GROUP BY
SELECT customer_id, sum(amount) FROM orders GROUP BY customer_id FOR UPDATE;
-- ERROR: FOR UPDATE is not allowed with GROUP BY clause

-- ❌ Window functions
SELECT id, row_number() OVER (ORDER BY id) FROM orders FOR UPDATE;
-- ERROR: FOR UPDATE is not allowed with window functions

-- ✅ Workaround: use a subquery
SELECT * FROM orders
WHERE id IN (
    SELECT id FROM orders WHERE status = 'pending' LIMIT 10
)
FOR UPDATE;
```

## Optimistic Locking

### Version Column Pattern

```sql
-- Table setup
CREATE TABLE documents (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT,
    version INT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Step 1: Read (no lock)
SELECT id, title, content, version FROM documents WHERE id = 42;
-- Returns: version = 5

-- Step 2: Application modifies the content in memory

-- Step 3: Write with version check (Compare-And-Swap)
UPDATE documents
SET content = 'new content',
    version = version + 1,
    updated_at = now()
WHERE id = 42 AND version = 5;
-- If another transaction updated the row (version is now 6),
-- this UPDATE matches 0 rows → application detects the conflict

-- Step 4: Check result
-- If affected_rows = 0 → CONFLICT → re-read and retry
-- If affected_rows = 1 → SUCCESS
```

### Why Not Use updated_at Instead of version?

```sql
-- ❌ Timestamp-based optimistic locking has a subtle bug:
UPDATE documents
SET content = 'new', updated_at = now()
WHERE id = 42 AND updated_at = '2024-01-15 10:30:00.123456';

-- Problem 1: Clock resolution
-- If two transactions start within the same microsecond,
-- they get the same timestamp → false success

-- Problem 2: Clock drift in replicas
-- Application might read from replica (stale timestamp)
-- and write to primary → version mismatch even without real conflict

-- ✅ Integer version is deterministic, monotonic, and has no resolution issues
```

### Optimistic Locking in JPA / Spring Data

```java
@Entity
public class Document {
    @Id @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    private String title;
    private String content;

    @Version  // JPA manages version automatically
    private Long version;
}

// Service layer MUST handle OptimisticLockException
@Service
public class DocumentService {

    @Retryable(
        retryFor = OptimisticLockingFailureException.class,
        maxAttempts = 3,
        backoff = @Backoff(delay = 50, multiplier = 2)
    )
    @Transactional
    public void updateDocument(Long id, String newContent) {
        Document doc = documentRepository.findById(id)
            .orElseThrow(() -> new NotFoundException("Document not found"));
        doc.setContent(newContent);
        documentRepository.save(doc);
        // If a concurrent update incremented the version,
        // Hibernate detects the mismatch and throws OptimisticLockException
        // @Retryable catches it and retries the entire method
    }
}
```

### Optimistic Locking at the Database Level (PostgreSQL REPEATABLE READ)

PostgreSQL provides optimistic concurrency control **natively** at REPEATABLE READ and SERIALIZABLE isolation levels:

```sql
-- Transaction 1:
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ;
SELECT balance FROM accounts WHERE id = 1;  -- sees 1000

-- Transaction 2 (concurrent):
BEGIN;
UPDATE accounts SET balance = 500 WHERE id = 1;
COMMIT;

-- Transaction 1 (continues):
UPDATE accounts SET balance = 1200 WHERE id = 1;
-- ERROR: could not serialize access due to concurrent update
-- PostgreSQL detected that T2 modified the row that T1 read
-- T1 must ROLLBACK and RETRY

-- This is essentially optimistic locking at the database level:
-- no explicit version column needed, but you MUST implement retry logic!
```

## Decision Matrix: When to Use Which

| Scenario | Recommended | Why |
|----------|-------------|-----|
| Bank transfer between accounts | **Pessimistic** (FOR UPDATE) | High contention on hot rows; incorrect balance is unacceptable |
| Editing a CMS article | **Optimistic** (version column) | Low contention; can show "conflict" UI to user |
| Inventory deduction (e-commerce) | **Pessimistic** (FOR UPDATE) | Overselling is catastrophic; contention is real during flash sales |
| User profile update | **Optimistic** (version column) | Very low contention; users rarely update simultaneously |
| Job queue processing | **Pessimistic** (FOR UPDATE SKIP LOCKED) | Multiple workers competing for the same jobs |
| Distributed cache invalidation | **Optimistic** (CAS with Redis or version) | High throughput needed; stale reads are tolerable briefly |
| Analytics counter increment | **Neither** — use atomic UPDATE | `UPDATE counters SET value = value + 1` avoids both |

> **Rule of thumb**: If the consequence of a conflict is **data loss or financial error**, use pessimistic locking. If the consequence is a **user-visible retry or merge**, use optimistic locking. If you can restructure the query to be **atomic** (no read-modify-write), do that instead.



# Chapter 5: Isolation Levels — PostgreSQL Deep Dive

## The Standard and the Reality

The SQL standard defines four isolation levels. PostgreSQL implements three of them genuinely (`READ COMMITTED`, `REPEATABLE READ`, `SERIALIZABLE`) and **aliases** `READ UNCOMMITTED` to `READ COMMITTED`. More importantly, PostgreSQL's implementation of each level is fundamentally different from lock-based databases.

## READ COMMITTED — The PostgreSQL Default

### How It Works Internally

Each **SQL statement** (not transaction) gets a fresh **snapshot** of the database. A snapshot is a point-in-time view defined by:

```
Snapshot definition:
┌──────────────────────────────────────────────────────────┐
│  xmin        = oldest active transaction ID              │
│  xmax        = next transaction ID to be assigned        │
│  xip_list[]  = list of in-progress transaction IDs       │
│                                                          │
│  A row is visible if:                                    │
│  1. row.xmin is committed AND row.xmin < snapshot.xmax   │
│     AND row.xmin NOT IN snapshot.xip_list                │
│  2. AND (row.xmax is not set, OR row.xmax is aborted,   │
│     OR row.xmax is in-progress, OR row.xmax > snap.xmax)│
└──────────────────────────────────────────────────────────┘
```

### READ COMMITTED Behavior in Detail

```sql
-- SESSION 1: Demonstrating statement-level snapshot
BEGIN;

-- Statement 1: sees all currently committed data
SELECT count(*) FROM orders WHERE status = 'pending';
-- Returns: 50

-- Meanwhile, SESSION 2 inserts and commits 10 new pending orders

-- Statement 2: gets a NEW snapshot — sees SESSION 2's committed changes
SELECT count(*) FROM orders WHERE status = 'pending';
-- Returns: 60  (non-repeatable read!)
```

### The Read-Committed UPDATE Anomaly

One of the most subtle behaviors. When an `UPDATE` encounters a row that was modified by a concurrent committed transaction, it **re-evaluates the WHERE clause** against the new version:

```sql
-- Table: accounts (id=1, balance=1000, status='active')

-- SESSION 1:
BEGIN;
UPDATE accounts SET balance = balance - 100 WHERE status = 'active' AND id = 1;
-- Locks row id=1, balance becomes 900
-- Not committed yet

-- SESSION 2:
BEGIN;
UPDATE accounts SET balance = balance - 200 WHERE status = 'active' AND id = 1;
-- Row is locked by Session 1 → Session 2 WAITS here

-- SESSION 1:
UPDATE accounts SET status = 'frozen' WHERE id = 1;
COMMIT;
-- balance = 900, status = 'frozen'

-- SESSION 2 (now unblocked):
-- PostgreSQL re-checks the WHERE clause against the NEW committed version
-- New version: balance=900, status='frozen'
-- WHERE status = 'active' → FALSE!
-- Session 2's UPDATE affects 0 rows (silently skipped!)

-- This is correct behavior per the spec, but surprising if you expect
-- Session 2's UPDATE to apply to balance=900
```

> **Production impact**: If your UPDATE's WHERE clause involves columns that other transactions might change, you may silently skip rows you intended to update. Use REPEATABLE READ or explicit locking if this matters.

### Configuration

```sql
-- Per transaction:
BEGIN TRANSACTION ISOLATION LEVEL READ COMMITTED;
-- or
BEGIN;
SET TRANSACTION ISOLATION LEVEL READ COMMITTED;

-- Per session (all subsequent transactions):
SET default_transaction_isolation = 'read committed';

-- Server-wide (postgresql.conf):
-- default_transaction_isolation = 'read committed'  (this is the default)

-- Check current level:
SHOW transaction_isolation;
```

## REPEATABLE READ — Snapshot Isolation

### How It Works

The snapshot is taken at the **first statement** in the transaction (not at BEGIN). All subsequent reads see this same snapshot, regardless of what other transactions commit.

```sql
-- SESSION 1:
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ;

-- This statement creates the snapshot
SELECT balance FROM accounts WHERE id = 1;  -- returns 1000

-- SESSION 2 (concurrent):
UPDATE accounts SET balance = 500 WHERE id = 1;
COMMIT;

-- SESSION 1 (continues):
SELECT balance FROM accounts WHERE id = 1;  -- STILL returns 1000
-- The snapshot is frozen — Session 2's commit is invisible

COMMIT;
```

### Serialization Failure on Write Conflicts

PostgreSQL detects when a REPEATABLE READ transaction tries to modify a row that was concurrently modified:

```sql
-- SESSION 1:
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ;
SELECT balance FROM accounts WHERE id = 1;  -- returns 1000

-- SESSION 2:
UPDATE accounts SET balance = 500 WHERE id = 1;
COMMIT;

-- SESSION 1:
UPDATE accounts SET balance = 1200 WHERE id = 1;
-- ERROR: could not serialize access due to concurrent update
-- SQLSTATE: 40001

-- Session 1 MUST rollback and retry the entire transaction
ROLLBACK;
```

### What REPEATABLE READ Does NOT Prevent

Despite its name, PostgreSQL's REPEATABLE READ (technically Snapshot Isolation) does **not** prevent **write skew**:

```sql
-- The classic on-call doctor example
-- Constraint: at least 1 doctor must be on-call
-- Currently: Alice (on_call=true), Bob (on_call=true)

-- SESSION 1 (REPEATABLE READ):
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ;
SELECT count(*) FROM doctors WHERE on_call = true;  -- returns 2
-- "Safe to go off call"
UPDATE doctors SET on_call = false WHERE name = 'Alice';

-- SESSION 2 (REPEATABLE READ):
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ;
SELECT count(*) FROM doctors WHERE on_call = true;  -- returns 2 (snapshot!)
-- "Safe to go off call"
UPDATE doctors SET on_call = false WHERE name = 'Bob';

-- Both sessions COMMIT successfully!
-- Result: 0 doctors on call — constraint violated!

-- REPEATABLE READ didn't catch this because the two transactions
-- wrote to DIFFERENT rows. No write-write conflict = no error.
-- Only SERIALIZABLE would catch this.
```

### Locking Reads at REPEATABLE READ (Concurrent Update Error)

In some databases (like MySQL/InnoDB), locking reads bypass the snapshot to see the latest data. **In PostgreSQL, this is not true.** If you try to lock a row that was updated by a concurrent transaction, PostgreSQL will throw an error rather than bypassing the snapshot:

```sql
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ;

-- Plain SELECT: reads from frozen snapshot
SELECT * FROM orders WHERE id = 1;  -- say it sees amount = 100

-- Meanwhile, Session 2 updates id=1 to amount=200 and commits.

-- Locking SELECT:
SELECT * FROM orders WHERE id = 1 FOR UPDATE;
-- ERROR: could not serialize access due to concurrent update
```

This strict adherence to the snapshot means you **never** see inconsistent data in `REPEATABLE READ` in PostgreSQL, but you must be prepared to catch serialization errors and retry the transaction.
```

## SERIALIZABLE — True Serializability via SSI

### How PostgreSQL's SSI Works

PostgreSQL's SERIALIZABLE level uses **Serializable Snapshot Isolation (SSI)**, a cutting-edge algorithm published by Cahill et al., 2008. It is fundamentally different from traditional lock-based serializable implementations (SQL Server) or gap-locking approaches (MySQL/InnoDB).

```
Traditional (SQL Server / MySQL):
  Readers acquire shared locks → block writers
  Writers acquire exclusive locks → block readers AND writers
  Result: low concurrency but prevents all anomalies

PostgreSQL SSI:
  Readers take MVCC snapshots → NEVER block writers
  Writers acquire row locks → only block other writers on the SAME row
  SSI tracks read-write dependencies between transactions
  If a dangerous pattern (rw-antidependency cycle) is detected → ABORT one txn
  Result: high concurrency but requires application-level RETRY logic
```

### The Dependencies SSI Tracks

SSI tracks three types of dependencies between transactions:

```
1. WR-dependency (write-read):
   T1 writes a row, T2 reads that row (from T1's committed version)

2. WW-dependency (write-write):
   T1 writes a row, T2 also writes that row

3. RW-antidependency (read-write):
   T1 reads a row (from snapshot), T2 writes to that row
   T1's snapshot "missed" T2's write

SSI detects cycles containing RW-antidependencies.
If found: one transaction is aborted with serialization_failure.
```

### SSI in Practice

```sql
-- Write skew prevention (the on-call doctor problem)
-- SESSION 1:
BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE;
SELECT count(*) FROM doctors WHERE on_call = true;
UPDATE doctors SET on_call = false WHERE name = 'Alice';
COMMIT;

-- SESSION 2 (concurrent):
BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE;
SELECT count(*) FROM doctors WHERE on_call = true;
UPDATE doctors SET on_call = false WHERE name = 'Bob';
COMMIT;
-- ERROR: could not serialize access due to read/write dependencies among transactions
-- DETAIL: Reason code: Canceled on identification as a pivot, during commit attempt.
-- HINT: The transaction might succeed if retried.
```

### Essential Retry Logic

```java
// You MUST implement retry logic when using SERIALIZABLE
@Service
public class DoctorService {

    private static final int MAX_RETRIES = 5;

    @Transactional(isolation = Isolation.SERIALIZABLE)
    public void goOffCall(String doctorName) {
        long onCallCount = doctorRepo.countByOnCallTrue();
        if (onCallCount <= 1) {
            throw new BusinessException("Cannot go off call: you're the last one");
        }
        doctorRepo.setOnCall(doctorName, false);
    }

    // Wrapper with retry
    public void goOffCallWithRetry(String doctorName) {
        for (int attempt = 1; attempt <= MAX_RETRIES; attempt++) {
            try {
                goOffCall(doctorName);
                return; // success
            } catch (CannotSerializeTransactionException e) {
                if (attempt == MAX_RETRIES) throw e;
                // Exponential backoff with jitter
                long delay = (long) (Math.pow(2, attempt) * 50 * (0.5 + Math.random()));
                Thread.sleep(delay);
            }
        }
    }
}
```

### SSI Performance Considerations

```sql
-- SSI uses predicate locks (SIReadLock) to track reads.
-- These are stored in shared memory and consume resources.

-- Check current predicate locks:
SELECT * FROM pg_locks WHERE locktype = 'relation' AND mode = 'SIReadLock';

-- Tuning parameters:
SHOW max_pred_locks_per_transaction;   -- default 64
SHOW max_pred_locks_per_relation;      -- default -2 (auto)
SHOW max_pred_locks_per_page;          -- default 2

-- When predicate locks exceed the per-page limit, PostgreSQL
-- escalates to a relation-level predicate lock.
-- This is safe (no correctness issue) but may cause
-- false-positive serialization failures.
```

### Comparison Across Isolation Levels

| Anomaly             | READ COMMITTED | REPEATABLE READ | SERIALIZABLE |
|---------------------|:--------------:|:---------------:|:------------:|
| Dirty Read          | ❌ Prevented   | ❌ Prevented    | ❌ Prevented |
| Non-Repeatable Read | ✅ Possible    | ❌ Prevented    | ❌ Prevented |
| Phantom Read        | ✅ Possible    | ❌ Prevented    | ❌ Prevented |
| Lost Update         | ✅ Possible    | ❌ Detected*    | ❌ Detected  |
| Write Skew          | ✅ Possible    | ✅ Possible     | ❌ Detected  |

*UPDATE or SELECT FOR UPDATE raises error: "could not serialize access due to concurrent update"

### Which Level Should You Use?

```
Decision guide:
─────────────────────────────────────────────────
READ COMMITTED (default): Use for 90% of workloads.
  ✅ Simple mental model per-statement
  ✅ No retry logic needed for read anomalies
  ✅ Use explicit locking (FOR UPDATE) when you need row-level exclusivity
  ⚠️  Be aware of non-repeatable reads in multi-statement transactions

REPEATABLE READ: Use when you need consistent reads across a transaction.
  ✅ Report generation (all queries see the same snapshot)
  ✅ Multi-step validation (check → act without stale reads)
  ⚠️  Must handle serialization_failure on write conflicts
  ⚠️  Does NOT prevent write skew

SERIALIZABLE: Use when correctness requires preventing ALL anomalies.
  ✅ Financial calculations, constraint enforcement involving reads
  ✅ When you can't prevent write skew with explicit locks
  ⚠️  MUST implement retry logic
  ⚠️  Higher overhead (predicate lock tracking)
  ⚠️  Higher abort rate under contention
─────────────────────────────────────────────────
```



# Chapter 6: MVCC — PostgreSQL's Concurrency Engine

## What MVCC Is

Multi-Version Concurrency Control is the mechanism that allows **readers to never block writers, and writers to never block readers**. Instead of using locks to control read access, PostgreSQL keeps **multiple physical versions** of each row and lets each transaction see the version appropriate to its snapshot.

## The Tuple Lifecycle

Every row in PostgreSQL (called a "tuple") has a lifecycle managed by hidden system columns:

```
┌─────────────────────────────────────────────────────────────────┐
│                    PostgreSQL Tuple Header                       │
├─────────────┬───────────────────────────────────────────────────┤
│  t_xmin     │  XID of the transaction that CREATED this version │
│  t_xmax     │  XID of the transaction that DELETED/LOCKED it    │
│  t_cid      │  Command ID within the creating transaction       │
│  t_ctid     │  Physical location (block number, offset)         │
│  t_infomask │  Status flags (committed, aborted, locked, etc.)  │
│  t_hoff     │  Offset to user data                              │
├─────────────┴───────────────────────────────────────────────────┤
│                    User Data (columns)                           │
└─────────────────────────────────────────────────────────────────┘
```

### How INSERT Works

```sql
INSERT INTO accounts (id, balance) VALUES (1, 1000);
-- Creates a new tuple:
-- t_xmin = current_transaction_id (e.g., 100)
-- t_xmax = 0 (no deleter)
-- data = {id: 1, balance: 1000}
```

```
Heap page after INSERT:
┌───────────────────────────────────────┐
│ Tuple: xmin=100, xmax=0, data={1000} │  ← only version
└───────────────────────────────────────┘
```

### How UPDATE Works (Critical!)

**PostgreSQL does NOT update tuples in-place.** Each UPDATE creates a **new tuple** and marks the old one as dead:

```sql
UPDATE accounts SET balance = 500 WHERE id = 1;
-- Step 1: Old tuple: set t_xmax = current_xid (e.g., 101)
-- Step 2: New tuple: t_xmin = 101, t_xmax = 0, data = {id: 1, balance: 500}
-- Step 3: Old tuple's t_ctid points to new tuple's location
```

```
Heap page after UPDATE:
┌───────────────────────────────────────┐
│ Tuple A: xmin=100, xmax=101 (DEAD)   │  ← old version
│   t_ctid → Tuple B                   │
│ Tuple B: xmin=101, xmax=0  (LIVE)    │  ← current version
│   data = {id: 1, balance: 500}       │
└───────────────────────────────────────┘
```

### How DELETE Works

```sql
DELETE FROM accounts WHERE id = 1;
-- Just sets t_xmax = current_xid on the current tuple
-- The tuple remains on disk until VACUUM removes it
```

### How SELECT Determines Visibility

```sql
-- Transaction 200 runs:
SELECT * FROM accounts WHERE id = 1;

-- For each tuple version found:
--   1. Is t_xmin committed? (check pg_xact / clog)
--   2. Is t_xmin visible to my snapshot?
--   3. Is t_xmax set? If yes:
--      a. Is t_xmax committed?
--      b. Is t_xmax visible to my snapshot?
--      c. If t_xmax is committed AND visible → tuple is DEAD to me
--   4. If tuple passes all checks → VISIBLE, include in result
```

## Examining MVCC Internals

```sql
-- See the hidden system columns
SELECT xmin, xmax, ctid, * FROM accounts WHERE id = 1;
-- xmin: creating transaction ID
-- xmax: deleting/locking transaction ID (0 = not deleted/locked)
-- ctid: physical location as (page_number, tuple_offset)

-- Example output:
--  xmin  | xmax | ctid  | id | balance
-- -------+------+-------+----+---------
--  100   |  0   | (0,1) |  1 |    1000

-- After an UPDATE:
--  xmin  | xmax | ctid  | id | balance
-- -------+------+-------+----+---------
--  101   |  0   | (0,2) |  1 |     500

-- The old version (xmin=100, xmax=101) is still on disk
-- but invisible to most transactions
```

### Viewing Transaction Status

```sql
-- What's your current transaction ID?
SELECT txid_current();  -- PostgreSQL 12 and earlier
SELECT pg_current_xact_id();  -- PostgreSQL 13+

-- What's in your current snapshot?
SELECT txid_current_snapshot();  -- e.g., '100:105:100,102'
-- Format: xmin:xmax:xip_list
-- xmin=100: oldest active transaction
-- xmax=105: next XID to be assigned
-- xip_list=100,102: currently in-progress transactions

-- Is a specific transaction visible to you?
SELECT txid_visible_in_snapshot('101', txid_current_snapshot());
-- Returns true/false
```

## Table Bloat — The MVCC Trade-off

Since UPDATE creates a new tuple without removing the old one, tables accumulate **dead tuples** (also called "dead rows" or "bloat"). This is the primary operational challenge of PostgreSQL's MVCC.

### Measuring Bloat

```sql
-- Quick dead tuple count
SELECT
    schemaname,
    relname,
    n_live_tup,
    n_dead_tup,
    CASE WHEN n_live_tup > 0
         THEN round(100.0 * n_dead_tup / n_live_tup, 1)
         ELSE 0 END AS dead_pct,
    last_autovacuum,
    last_autoanalyze
FROM pg_stat_user_tables
ORDER BY n_dead_tup DESC;

-- Detailed bloat estimate using pgstattuple extension
CREATE EXTENSION IF NOT EXISTS pgstattuple;
SELECT * FROM pgstattuple('accounts');
-- Returns: table_len, tuple_count, tuple_len, tuple_percent,
--          dead_tuple_count, dead_tuple_len, dead_tuple_percent, free_space, free_percent
```

### VACUUM — The Cleanup Process

```sql
-- Standard VACUUM: marks dead tuples as reusable space
-- Does NOT return space to the OS
-- Does NOT block reads or writes
VACUUM accounts;

-- VACUUM VERBOSE: shows what happened
VACUUM VERBOSE accounts;
-- INFO: "accounts": removed 1500 dead row versions in 20 pages
-- DETAIL: CPU: user: 0.00 s, system: 0.00 s, elapsed: 0.01 s

-- VACUUM FULL: rewrites the entire table, reclaiming disk space
-- BLOCKS ALL ACCESS (acquires ACCESS EXCLUSIVE lock)
-- Only use for massive bloat recovery, during maintenance windows
VACUUM FULL accounts;

-- VACUUM FREEZE: marks old tuples as "frozen" (permanently visible)
-- Prevents transaction ID wraparound (see below)
VACUUM FREEZE accounts;
```

### Autovacuum Configuration

```sql
-- Key autovacuum settings (postgresql.conf)
SHOW autovacuum;                        -- on (default)
SHOW autovacuum_vacuum_threshold;       -- 50 (min dead tuples before vacuum)
SHOW autovacuum_vacuum_scale_factor;    -- 0.2 (20% of table must be dead)
SHOW autovacuum_naptime;                -- 1min (check interval)

-- Trigger formula:
-- vacuum_threshold + vacuum_scale_factor * n_live_tup
-- For a 1M row table: 50 + 0.2 * 1,000,000 = 200,050 dead tuples before vacuum

-- For high-write tables, reduce the scale factor:
ALTER TABLE hot_table SET (
    autovacuum_vacuum_scale_factor = 0.01,   -- 1% instead of 20%
    autovacuum_vacuum_threshold = 1000,
    autovacuum_analyze_scale_factor = 0.005
);
```

## Transaction ID Wraparound — The Silent Killer

PostgreSQL uses 32-bit transaction IDs (XIDs). With ~4 billion possible values, a busy database can theoretically exhaust them. When this happens, PostgreSQL enters **forced shutdown** to prevent data corruption.

```sql
-- Check how close you are to wraparound
SELECT
    datname,
    age(datfrozenxid) AS xid_age,
    -- Standard warning threshold: 200M. Danger: > 1B
    CASE WHEN age(datfrozenxid) > 1000000000 THEN '🔴 DANGER'
         WHEN age(datfrozenxid) > 200000000  THEN '🟡 WARNING'
         ELSE '🟢 OK' END AS status
FROM pg_database
ORDER BY age(datfrozenxid) DESC;

-- Check per-table:
SELECT
    c.oid::regclass AS table_name,
    age(c.relfrozenxid) AS xid_age,
    pg_size_pretty(pg_table_size(c.oid)) AS table_size
FROM pg_class c
JOIN pg_namespace n ON c.relnamespace = n.oid
WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY age(c.relfrozenxid) DESC
LIMIT 20;
```

> **Prevention**: Autovacuum's FREEZE operation handles this automatically. If you see `age(datfrozenxid)` growing past 200 million, your autovacuum is falling behind. Fix the underlying cause (long-running transactions, autovacuum tuning, I/O bottleneck) before it becomes an emergency.

## HOT Updates — Mitigating MVCC Overhead

Heap-Only Tuples (HOT) is PostgreSQL's optimization to reduce the cost of UPDATE:

```
Normal UPDATE:
  1. Create new heap tuple
  2. Create new index entry pointing to new tuple
  3. Old index entries still point to old (dead) tuple

HOT UPDATE:
  1. Create new heap tuple ON THE SAME PAGE
  2. NO new index entry needed — old entry chains to new tuple via ctid
  3. Significantly fewer index writes, faster VACUUM
```

```sql
-- HOT updates happen when:
-- 1. Updated columns are NOT in any index
-- 2. New tuple fits on the same heap page

-- Check HOT update ratio:
SELECT
    relname,
    n_tup_upd,
    n_tup_hot_upd,
    CASE WHEN n_tup_upd > 0
         THEN round(100.0 * n_tup_hot_upd / n_tup_upd, 1)
         ELSE 0 END AS hot_update_pct
FROM pg_stat_user_tables
ORDER BY n_tup_upd DESC;

-- Aim for > 90% HOT update ratio on frequently updated tables
-- If low, consider:
-- 1. Removing unnecessary indexes on frequently updated columns
-- 2. Increasing fillfactor to leave room for HOT updates:
ALTER TABLE accounts SET (fillfactor = 80);
-- Leaves 20% free space per page for HOT updates
```

## The Visibility Map and Index-Only Scans

The **visibility map** tracks which heap pages contain only all-visible tuples. This enables:

1. **Index-only scans**: If all requested columns are in the index AND the page is all-visible, PostgreSQL can answer the query from the index alone (no heap access)
2. **VACUUM optimization**: VACUUM can skip all-visible pages

```sql
-- Check visibility map coverage
CREATE EXTENSION IF NOT EXISTS pg_visibility;
SELECT * FROM pg_visibility_map_summary('accounts');
-- Returns: all_visible (pages fully visible), all_frozen (pages with frozen tuples)

-- Force an index-only scan
-- 1. Create a covering index
CREATE INDEX idx_accounts_balance ON accounts (id) INCLUDE (balance);
-- 2. Run VACUUM to populate visibility map
VACUUM accounts;
-- 3. Query using only indexed columns
EXPLAIN (ANALYZE, BUFFERS) SELECT id, balance FROM accounts WHERE id = 1;
-- Should show: "Index Only Scan" if visibility map is up to date
```



# Chapter 7: Deadlocks in PostgreSQL

## What Is a Deadlock?

A deadlock occurs when two or more transactions form a **circular wait** — each holds a lock the other needs, and none can proceed.

```
     T1 holds lock on Row A         T2 holds lock on Row B
     T1 wants lock on Row B  ──→    T2 wants lock on Row A
                    ↑                          │
                    └──────── CYCLE ────────────┘
     Neither can proceed. PostgreSQL MUST kill one.
```

## A Concrete Deadlock

```sql
-- Transfer money between accounts (WRONG way)

-- Session 1: Transfer $100 from Account 1 → Account 2
BEGIN;
UPDATE accounts SET balance = balance - 100 WHERE id = 1;
-- Acquires X lock on row id=1
-- ... application does some processing ...

-- Session 2: Transfer $50 from Account 2 → Account 1
BEGIN;
UPDATE accounts SET balance = balance - 50 WHERE id = 2;
-- Acquires X lock on row id=2

-- Session 1 continues:
UPDATE accounts SET balance = balance + 100 WHERE id = 2;
-- BLOCKED! Session 2 holds lock on row id=2. Session 1 waits.

-- Session 2 continues:
UPDATE accounts SET balance = balance + 50 WHERE id = 1;
-- BLOCKED! Session 1 holds lock on row id=1. Session 2 waits.

-- DEADLOCK DETECTED!
-- PostgreSQL detects the cycle and kills one session:
-- ERROR: deadlock detected
-- DETAIL: Process 12345 waits for ShareLock on transaction 67890;
--         blocked by process 67891.
--         Process 67891 waits for ShareLock on transaction 67889;
--         blocked by process 12345.
-- HINT: See server log for query details.
```

## How PostgreSQL Detects Deadlocks

### The Wait-For Graph

PostgreSQL maintains an in-memory **wait-for graph** (WFG). Nodes are transactions. Edges represent "Transaction A is waiting for Transaction B to release a lock." A **cycle** in this graph = deadlock.

### Detection Timing: `deadlock_timeout`

PostgreSQL uses a **two-phase** approach:

```
Phase 1: When a transaction can't acquire a lock, it SLEEPS for
         deadlock_timeout (default: 1 second).

Phase 2: If still waiting after deadlock_timeout, PostgreSQL builds
         the wait-for graph and checks for cycles.

Why two phases?
  - Most lock waits resolve quickly (< 1 second)
  - Building the WFG is expensive (requires examining all locks)
  - The two-phase approach avoids unnecessary WFG construction
```

```sql
-- View and tune deadlock_timeout
SHOW deadlock_timeout;          -- default: 1s
SET deadlock_timeout = '500ms'; -- detect faster (higher CPU cost)
SET deadlock_timeout = '5s';    -- detect slower (lower CPU cost)

-- For high-contention OLTP: consider 200ms-500ms
-- For analytical workloads: default 1s is fine
```

### Victim Selection

When PostgreSQL detects a deadlock, it aborts the transaction that **triggered the detection** — the one whose `deadlock_timeout` expired. The other transaction(s) in the cycle proceed normally.

> **Important**: Unlike SQL Server, PostgreSQL does NOT consider transaction "cost" when choosing the victim. It's purely "whoever triggered the check loses." This is simpler but means you can't control victim selection with priority hints.

## Prevention Strategies

### Strategy 1: Consistent Lock Ordering

The **most effective** deadlock prevention. Always acquire locks in the same global order.

```sql
-- CORRECT: Always lock the lower account ID first
CREATE OR REPLACE FUNCTION transfer_funds(
    from_id INT,
    to_id INT,
    amount NUMERIC
) RETURNS VOID AS $$
DECLARE
    first_id INT := LEAST(from_id, to_id);
    second_id INT := GREATEST(from_id, to_id);
    first_row accounts%ROWTYPE;
    second_row accounts%ROWTYPE;
BEGIN
    -- Always lock lower ID first, regardless of transfer direction
    SELECT * INTO first_row FROM accounts WHERE id = first_id FOR UPDATE;
    SELECT * INTO second_row FROM accounts WHERE id = second_id FOR UPDATE;

    -- Now perform the transfer
    UPDATE accounts SET balance = balance - amount WHERE id = from_id;
    UPDATE accounts SET balance = balance + amount WHERE id = to_id;
END;
$$ LANGUAGE plpgsql;

-- No matter which direction the transfer goes, locks are always
-- acquired in ascending ID order → deadlock impossible between
-- any two transfer operations.
```

### Strategy 2: Lock Timeout

```sql
-- Abort if a lock can't be acquired within the timeout
SET lock_timeout = '3s';

BEGIN;
SELECT * FROM accounts WHERE id = 1 FOR UPDATE;
-- If this can't acquire the lock in 3 seconds:
-- ERROR: canceling statement due to lock timeout

-- This doesn't prevent deadlocks, but limits the damage.
-- The application should retry with backoff.

-- Per-transaction override:
BEGIN;
SET LOCAL lock_timeout = '1s';  -- only for this transaction
SELECT * FROM accounts WHERE id = 1 FOR UPDATE;
COMMIT;
```

### Strategy 3: NOWAIT

```sql
-- Fail IMMEDIATELY if lock is not available
BEGIN;
SELECT * FROM accounts WHERE id = 1 FOR UPDATE NOWAIT;
-- If the row is locked: ERROR: could not obtain lock on row in relation "accounts"
-- Application must catch and retry

-- Use case: APIs where blocking = poor user experience
-- Better to fail fast and retry than to hold a connection waiting
```

### Strategy 4: Reduce Transaction Duration

```sql
-- BAD: Long transaction with external API call
BEGIN;
SELECT * FROM orders WHERE id = 42 FOR UPDATE;
-- ... make HTTP call to payment gateway (takes 2-5 seconds) ...
UPDATE orders SET status = 'paid' WHERE id = 42;
COMMIT;
-- Lock held for 2-5 seconds! High deadlock risk.

-- GOOD: Minimize lock-holding time
-- Step 1: External call OUTSIDE the transaction
payment_result = call_payment_gateway(order_id=42);

-- Step 2: Short transaction to update DB
BEGIN;
SELECT * FROM orders WHERE id = 42 FOR UPDATE;
UPDATE orders SET status = 'paid', payment_ref = payment_result.ref
WHERE id = 42;
COMMIT;
-- Lock held for milliseconds
```

### Strategy 5: Advisory Locks to Serialize Access

```sql
-- Use advisory locks to prevent deadlocks entirely
-- by serializing access to specific resources

BEGIN;
-- Lock based on a business key (e.g., customer ID)
-- This prevents any concurrent transaction for the same customer
SELECT pg_advisory_xact_lock(hashtext('customer_order_' || customer_id::text));

-- Now safe to modify multiple tables for this customer
-- without worrying about lock ordering
UPDATE accounts SET balance = balance - 100 WHERE customer_id = 42;
INSERT INTO transactions (customer_id, amount) VALUES (42, -100);
UPDATE customer_stats SET total_spent = total_spent + 100 WHERE id = 42;

COMMIT; -- advisory lock released
```

## Monitoring Deadlocks

### Enable Deadlock Logging

```sql
-- postgresql.conf
-- log_lock_waits = on
-- deadlock_timeout = 1s

-- When a deadlock occurs, the server log shows:
-- LOG: process 12345 detected deadlock while waiting for ShareLock
--      on transaction 67890 after 1000.123 ms
-- DETAIL: Process holding the lock: 67891. Wait queue: .
-- CONTEXT: while updating tuple (0,1) in relation "accounts"
-- STATEMENT: UPDATE accounts SET balance = balance + 100 WHERE id = 2;
```

### Query Lock Waits

```sql
-- Find all current lock waits
SELECT
    blocked.pid AS blocked_pid,
    blocked.usename AS blocked_user,
    blocked.query AS blocked_query,
    blocked.query_start AS blocked_since,
    now() - blocked.query_start AS wait_duration,
    blocking.pid AS blocking_pid,
    blocking.usename AS blocking_user,
    blocking.query AS blocking_query,
    blocking.state AS blocking_state
FROM pg_stat_activity blocked
JOIN pg_locks bl ON bl.pid = blocked.pid AND NOT bl.granted
JOIN pg_locks kl ON kl.transactionid = bl.transactionid AND kl.granted
JOIN pg_stat_activity blocking ON kl.pid = blocking.pid
WHERE blocked.pid != blocking.pid;
```

### PostgreSQL 14+ Lock Monitoring

```sql
-- pg_stat_activity improvements in PG14+
SELECT
    pid,
    wait_event_type,  -- 'Lock' for lock waits
    wait_event,       -- 'transactionid', 'relation', 'tuple', etc.
    state,
    query,
    now() - query_start AS duration
FROM pg_stat_activity
WHERE wait_event_type = 'Lock';
```

## Common Deadlock Patterns and Fixes

### Pattern 1: Foreign Key Deadlock

```sql
-- tables: orders (id, customer_id FK → customers.id)
-- Deadlock between INSERT into orders and UPDATE on customers

-- T1: INSERT INTO orders (customer_id, amount) VALUES (1, 100);
-- Acquires: X lock on new orders row + FOR KEY SHARE on customers id=1

-- T2: UPDATE customers SET name = 'New Name' WHERE id = 1;
-- Acquires: FOR NO KEY UPDATE on customers id=1 (compatible with FOR KEY SHARE)
-- BUT if the UPDATE touches a key column:
-- UPDATE customers SET id = 999 WHERE id = 1;
-- Acquires: FOR UPDATE on customers id=1 → CONFLICTS with FOR KEY SHARE!

-- Fix: Don't update primary keys. If you must, ensure no concurrent inserts
-- to child tables. Use ON UPDATE CASCADE if appropriate.
```

### Pattern 2: Index Scan Order Deadlock

```sql
-- Table with index: CREATE INDEX idx_amount ON orders(amount);

-- T1: UPDATE orders SET status = 'processed' WHERE amount > 100;
-- Scans index in ascending amount order: locks row(101), row(150), row(200)...

-- T2: UPDATE orders SET status = 'reviewed' WHERE amount > 50;
-- Also scans in ascending order but updates different rows...
-- If index scan order differs (e.g., different plans), deadlock!

-- Fix: This is rare in PostgreSQL (same index = same scan order)
-- But possible with different WHERE clauses hitting different indexes
-- Mitigation: batch updates, use SKIP LOCKED for queue-like patterns
```



# Chapter 8: Real-World Locking Patterns in PostgreSQL

## Pattern 1: Job Queue with SKIP LOCKED

The most elegant alternative to external message queues for moderate-throughput workloads. Multiple workers compete for tasks using database locks as the coordination mechanism.

### Basic Implementation

```sql
-- Table setup
CREATE TABLE jobs (
    id BIGSERIAL PRIMARY KEY,
    queue_name TEXT NOT NULL DEFAULT 'default',
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_jobs_fetchable ON jobs (queue_name, scheduled_at)
WHERE status = 'pending';

-- Worker fetches next job
BEGIN;
SELECT * FROM jobs
WHERE status = 'pending'
  AND queue_name = 'emails'
  AND scheduled_at <= now()
  AND attempts < max_attempts
ORDER BY scheduled_at
LIMIT 1
FOR UPDATE SKIP LOCKED;  -- Skip rows locked by other workers

-- If a row was returned, update it
UPDATE jobs SET status = 'processing', started_at = now(), attempts = attempts + 1
WHERE id = :fetched_id;
COMMIT;

-- Process the job outside the transaction...

-- On success:
UPDATE jobs SET status = 'completed', completed_at = now() WHERE id = :fetched_id;

-- On failure:
UPDATE jobs SET
    status = CASE WHEN attempts >= max_attempts THEN 'dead' ELSE 'pending' END,
    failed_at = now(),
    error_message = :error,
    scheduled_at = now() + (attempts * INTERVAL '1 minute')  -- exponential backoff
WHERE id = :fetched_id;
```

### Why SKIP LOCKED Over Other Approaches

```
Approach              | Behavior                    | Problem
─────────────────────────────────────────────────────────────
No locking            | Multiple workers grab same  | Double-processing
                      | job                         |
FOR UPDATE (blocking) | Workers queue up in order   | Serialized workers,
                      |                             | no parallelism
FOR UPDATE NOWAIT     | Workers fail on locked rows | Errors to handle, may
                      |                             | never get a job
FOR UPDATE SKIP LOCKED| Workers skip locked rows,   | ✅ True parallelism,
                      | grab the next available     | no errors, no duplicates
```

### Batch Fetching

```sql
-- Fetch up to 10 jobs at once (more efficient than 1 at a time)
BEGIN;
WITH grabbed AS (
    SELECT id FROM jobs
    WHERE status = 'pending'
      AND queue_name = 'emails'
      AND scheduled_at <= now()
    ORDER BY scheduled_at
    LIMIT 10
    FOR UPDATE SKIP LOCKED
)
UPDATE jobs SET status = 'processing', started_at = now()
FROM grabbed
WHERE jobs.id = grabbed.id
RETURNING jobs.*;
COMMIT;
```

## Pattern 2: Inventory Reservation (Preventing Overselling)

### The Naive Approach (BROKEN)

```sql
-- Two customers buy the last item simultaneously
-- Stock: 1

-- Customer A:
BEGIN;
SELECT stock FROM products WHERE id = 42;  -- sees stock = 1
-- Application: stock >= 1, allow purchase
UPDATE products SET stock = stock - 1 WHERE id = 42;
INSERT INTO order_items (order_id, product_id) VALUES (100, 42);
COMMIT;

-- Customer B (concurrent):
BEGIN;
SELECT stock FROM products WHERE id = 42;  -- ALSO sees stock = 1!
-- Application: stock >= 1, allow purchase
UPDATE products SET stock = stock - 1 WHERE id = 42;
INSERT INTO order_items (order_id, product_id) VALUES (101, 42);
COMMIT;

-- Result: stock = -1. We oversold!
```

### Solution 1: Pessimistic Lock

```sql
BEGIN;
SELECT stock FROM products WHERE id = 42 FOR UPDATE;
-- Row is locked — other buyers wait here

IF stock >= requested_quantity THEN
    UPDATE products SET stock = stock - requested_quantity WHERE id = 42;
    INSERT INTO order_items (order_id, product_id, quantity) VALUES (:order_id, 42, :qty);
ELSE
    RAISE EXCEPTION 'Insufficient stock';
END IF;
COMMIT;
```

### Solution 2: Atomic Check-and-Decrement (Lock-Free)

```sql
-- Single atomic statement — no explicit lock needed
UPDATE products
SET stock = stock - :quantity
WHERE id = 42 AND stock >= :quantity
RETURNING stock;

-- If RETURNING gives no results → insufficient stock
-- If it returns the new stock → success
-- No explicit lock needed — the UPDATE's implicit row lock suffices
-- This is the preferred approach for simple inventory
```

### Solution 3: CHECK Constraint (Database-Enforced)

```sql
ALTER TABLE products ADD CONSTRAINT stock_non_negative CHECK (stock >= 0);

-- Now this will fail automatically:
UPDATE products SET stock = stock - 1 WHERE id = 42;
-- If stock would go below 0:
-- ERROR: new row for relation "products" violates check constraint "stock_non_negative"
-- DETAIL: Failing row contains (..., -1, ...).
```

## Pattern 3: Upsert Race Conditions

### The INSERT ... ON CONFLICT Pattern

```sql
-- Count page views per URL
CREATE TABLE page_views (
    url TEXT PRIMARY KEY,
    view_count BIGINT NOT NULL DEFAULT 0,
    last_viewed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Atomic upsert — no locking concerns
INSERT INTO page_views (url, view_count, last_viewed_at)
VALUES ('https://example.com/page1', 1, now())
ON CONFLICT (url) DO UPDATE SET
    view_count = page_views.view_count + 1,
    last_viewed_at = now();

-- PostgreSQL handles the locking internally:
-- If no conflict: acquires row lock for INSERT
-- If conflict: acquires FOR UPDATE on existing row, then UPDATEs
```

### Upsert With Conditional Logic

```sql
-- Only update if the new data is "newer" (idempotent event processing)
INSERT INTO entity_state (entity_id, state, updated_at, event_seq)
VALUES (:id, :state, :updated_at, :seq)
ON CONFLICT (entity_id) DO UPDATE SET
    state = EXCLUDED.state,
    updated_at = EXCLUDED.updated_at,
    event_seq = EXCLUDED.event_seq
WHERE entity_state.event_seq < EXCLUDED.event_seq;
-- Skips the update if we've already processed a newer event
```

## Pattern 4: Read-Modify-Write with CTE

```sql
-- Transfer funds using a single atomic CTE (no explicit locking)
WITH debit AS (
    UPDATE accounts SET balance = balance - 100
    WHERE id = 1 AND balance >= 100
    RETURNING id, balance
),
credit AS (
    UPDATE accounts SET balance = balance + 100
    WHERE id = 2
    AND EXISTS (SELECT 1 FROM debit)  -- only credit if debit succeeded
    RETURNING id, balance
)
SELECT * FROM debit
UNION ALL
SELECT * FROM credit;

-- If debit fails (insufficient funds), credit is also skipped
-- No explicit FOR UPDATE needed — UPDATEs lock rows implicitly
-- All steps run in a single statement = single snapshot
```

## Pattern 5: Distributed Locking When PostgreSQL Isn't Enough

### When You Need External Locks

- Operations spanning **multiple databases** or services
- Coordinating **non-database resources** (file systems, APIs)
- **Cross-service** mutual exclusion

### Redis Distributed Lock

```java
// Using Redisson
RLock lock = redisson.getLock("payment-" + orderId);
try {
    if (lock.tryLock(5, 30, TimeUnit.SECONDS)) {
        try {
            processPayment(orderId);
        } finally {
            lock.unlock();
        }
    } else {
        throw new CouldNotAcquireLockException("Payment lock timeout");
    }
} catch (InterruptedException e) {
    Thread.currentThread().interrupt();
}
```

### Fencing Tokens — Preventing Stale Lock Holders

```
Problem:
  T1 acquires lock (TTL = 30s)
  T1 pauses (GC, network partition, process stall)
  Lock expires
  T2 acquires the SAME lock
  T1 resumes — thinks it still holds the lock!
  Both T1 and T2 are now writing concurrently

Solution: Fencing tokens
  Each lock acquisition returns a monotonically increasing token.
  The storage layer rejects writes with an older token.
```

```sql
-- PostgreSQL fencing token implementation
CREATE TABLE resources (
    id INT PRIMARY KEY,
    data TEXT,
    fence_token BIGINT NOT NULL DEFAULT 0
);

-- Writer must include its fence token
UPDATE resources SET data = :new_data, fence_token = :my_token
WHERE id = :id AND fence_token < :my_token;
-- If affected_rows = 0 → stale lock holder, write rejected
```

## Pattern 6: Preventing Duplicate Processing (Idempotency)

```sql
-- Using advisory locks + processed_events table
BEGIN;

-- Step 1: Acquire advisory lock on event ID (prevents concurrent processing)
SELECT pg_advisory_xact_lock(hashtext(:event_id));

-- Step 2: Check if already processed
SELECT 1 FROM processed_events WHERE event_id = :event_id;
-- If found → skip, COMMIT

-- Step 3: Process the event
INSERT INTO processed_events (event_id, processed_at) VALUES (:event_id, now());
-- ... business logic ...

COMMIT;  -- advisory lock released

-- The advisory lock prevents two workers from processing the same event
-- simultaneously, even before the processed_events row is visible.
```



# Chapter 9: Monitoring, Debugging & Performance Tuning

## The Essential Monitoring Queries

### Query 1: What's Locked Right Now?

```sql
-- Complete lock overview: who's waiting for whom
SELECT
    COALESCE(blockingl.relation::regclass::text, blockingl.locktype) AS locked_item,
    now() - blockeda.query_start AS waiting_duration,
    blockeda.pid AS blocked_pid,
    blockeda.query AS blocked_query,
    blockedl.mode AS blocked_mode,
    blockinga.pid AS blocking_pid,
    blockinga.query AS blocking_query,
    blockingl.mode AS blocking_mode
FROM pg_locks blockedl
JOIN pg_stat_activity blockeda ON blockedl.pid = blockeda.pid
JOIN pg_locks blockingl ON (
    blockingl.transactionid = blockedl.transactionid
    OR (blockingl.relation = blockedl.relation
        AND blockingl.locktype = blockedl.locktype)
)
JOIN pg_stat_activity blockinga ON blockingl.pid = blockinga.pid
WHERE NOT blockedl.granted
  AND blockeda.pid != blockinga.pid
ORDER BY waiting_duration DESC;
```

### Query 2: All Locks on a Specific Table

```sql
-- Show all locks held on the 'orders' table
SELECT
    l.locktype,
    l.mode,
    l.granted,
    l.pid,
    a.usename,
    a.query,
    a.state,
    a.query_start,
    now() - a.query_start AS duration
FROM pg_locks l
JOIN pg_stat_activity a ON l.pid = a.pid
WHERE l.relation = 'orders'::regclass
ORDER BY l.granted, a.query_start;
```

### Query 3: Long-Running Transactions (Lock Holders)

```sql
-- Transactions open for more than 5 minutes
-- These are the #1 cause of lock contention and table bloat
SELECT
    pid,
    usename,
    state,
    query,
    xact_start,
    now() - xact_start AS transaction_duration,
    query_start,
    now() - query_start AS query_duration,
    wait_event_type,
    wait_event
FROM pg_stat_activity
WHERE state != 'idle'
  AND xact_start IS NOT NULL
  AND now() - xact_start > INTERVAL '5 minutes'
ORDER BY xact_start;
```

### Query 4: Idle-in-Transaction Sessions

```sql
-- These are the MOST DANGEROUS: open transactions doing nothing
-- They hold locks AND prevent VACUUM from cleaning up dead tuples
SELECT
    pid,
    usename,
    application_name,
    state,
    xact_start,
    now() - xact_start AS idle_in_txn_duration,
    query AS last_query
FROM pg_stat_activity
WHERE state = 'idle in transaction'
  AND xact_start IS NOT NULL
ORDER BY xact_start;
```

### Query 5: Advisory Lock Usage

```sql
SELECT
    l.pid,
    l.classid,
    l.objid,
    l.objsubid,
    l.mode,
    l.granted,
    a.usename,
    a.application_name,
    a.query
FROM pg_locks l
JOIN pg_stat_activity a ON l.pid = a.pid
WHERE l.locktype = 'advisory'
ORDER BY l.granted, l.pid;
```

## Essential Configuration Parameters

### Lock Timeouts

```sql
-- lock_timeout: Max time to wait for any lock (table or row)
SET lock_timeout = '5s';        -- per session
SET LOCAL lock_timeout = '5s';  -- per transaction only
-- Default: 0 (wait forever) — DANGEROUS in production!

-- Recommendation: Set globally in postgresql.conf
-- lock_timeout = '10s'  (for OLTP applications)
-- lock_timeout = '30s'  (for batch/ETL jobs)

-- statement_timeout: Max time for any statement to execute
SET statement_timeout = '30s';
-- If a statement runs longer than this, it's killed
-- This catches runaway queries that might hold locks

-- idle_in_transaction_session_timeout: Kill idle-in-transaction sessions
SET idle_in_transaction_session_timeout = '5min';
-- CRITICAL: Set this in production!
-- An idle-in-transaction session holds locks indefinitely
-- and prevents VACUUM from cleaning dead tuples

-- deadlock_timeout: Time to wait before checking for deadlocks
SET deadlock_timeout = '1s';  -- default
-- For high-contention systems, consider 200ms-500ms
```

### Configuration Recommendations by Workload

```
OLTP (Web Application):
  lock_timeout = '5s'
  statement_timeout = '30s'
  idle_in_transaction_session_timeout = '1min'
  deadlock_timeout = '500ms'

Batch/ETL:
  lock_timeout = '60s'
  statement_timeout = '10min'
  idle_in_transaction_session_timeout = '5min'
  deadlock_timeout = '1s'

Analytics/Reporting (Read-Heavy):
  lock_timeout = '30s'
  statement_timeout = '5min'
  idle_in_transaction_session_timeout = '10min'
  deadlock_timeout = '1s'
```

## Debugging Lock Contention Step by Step

### Step 1: Identify the Symptom

```sql
-- Check for lock-related wait events
SELECT
    wait_event_type,
    wait_event,
    count(*) AS sessions_waiting
FROM pg_stat_activity
WHERE wait_event_type IS NOT NULL
GROUP BY wait_event_type, wait_event
ORDER BY count(*) DESC;

-- Lock-related wait events:
-- wait_event_type = 'Lock', wait_event = 'transactionid' → row lock wait
-- wait_event_type = 'Lock', wait_event = 'relation' → table lock wait
-- wait_event_type = 'Lock', wait_event = 'tuple' → hot tuple contention
-- wait_event_type = 'Lock', wait_event = 'advisory' → advisory lock wait
```

### Step 2: Find the Blocking Chain

```sql
-- PostgreSQL 14+: use pg_blocking_pids() function
SELECT
    pid,
    pg_blocking_pids(pid) AS blocked_by,
    query,
    wait_event_type,
    wait_event,
    state
FROM pg_stat_activity
WHERE cardinality(pg_blocking_pids(pid)) > 0;

-- Recursive lock chain (who's blocking who's blocking who)
WITH RECURSIVE lock_chain AS (
    SELECT pid, pg_blocking_pids(pid) AS blockers, query, 1 AS depth
    FROM pg_stat_activity
    WHERE cardinality(pg_blocking_pids(pid)) > 0

    UNION ALL

    SELECT a.pid, pg_blocking_pids(a.pid), a.query, lc.depth + 1
    FROM pg_stat_activity a
    JOIN lock_chain lc ON a.pid = ANY(lc.blockers)
    WHERE depth < 10
)
SELECT * FROM lock_chain ORDER BY depth;
```

### Step 3: Resolve

```sql
-- Option 1: Wait for the blocking transaction to finish (preferred)

-- Option 2: Cancel the blocking query (query stops, txn survives)
SELECT pg_cancel_backend(:blocking_pid);

-- Option 3: Terminate the blocking session (connection killed, txn rolled back)
SELECT pg_terminate_backend(:blocking_pid);

-- Be careful: terminating a session rolls back its transaction,
-- which means any work it did is lost. Use pg_cancel_backend first.
```

## Performance Optimization Techniques

### Technique 1: Reduce Lock Scope with Partial Indexes

```sql
-- Instead of locking against a full table scan:
SELECT * FROM orders WHERE status = 'pending' FOR UPDATE;
-- Without index: sequential scan, briefly "touches" all rows

-- Add a partial index:
CREATE INDEX idx_orders_pending ON orders(created_at) WHERE status = 'pending';
-- Now the FOR UPDATE only touches the index entries for pending orders
-- Much faster, fewer rows examined
```

### Technique 2: Micro-Batch Updates

```sql
-- BAD: Update 1 million rows in one transaction
UPDATE events SET processed = true WHERE processed = false;
-- Locks 1M rows, generates 1M dead tuples, blocks concurrent UPDATEs

-- GOOD: Process in batches
DO $$
DECLARE rows_updated INT;
BEGIN
  LOOP
    WITH batch AS (
      SELECT id FROM events
      WHERE processed = false
      LIMIT 5000
      FOR UPDATE SKIP LOCKED
    )
    UPDATE events SET processed = true
    FROM batch WHERE events.id = batch.id;

    GET DIAGNOSTICS rows_updated = ROW_COUNT;
    RAISE NOTICE 'Updated % rows', rows_updated;
    EXIT WHEN rows_updated = 0;

    -- Brief sleep to allow autovacuum and other transactions
    PERFORM pg_sleep(0.1);
  END LOOP;
END $$;
```

### Technique 3: Use CONCURRENTLY for Index Operations

```sql
-- BAD: Blocks all writes during index creation
CREATE INDEX idx_orders_date ON orders(created_at);
-- Acquires SHARE lock on the table → blocks INSERT, UPDATE, DELETE

-- GOOD: Non-blocking index creation
CREATE INDEX CONCURRENTLY idx_orders_date ON orders(created_at);
-- Acquires SHARE UPDATE EXCLUSIVE → doesn't block reads OR writes
-- Takes longer (multiple table scans) but zero downtime

-- Same for REINDEX:
REINDEX INDEX CONCURRENTLY idx_orders_date;  -- PostgreSQL 12+

-- And for DROP INDEX:
DROP INDEX CONCURRENTLY idx_orders_date;
```

### Technique 4: Online DDL Patterns

```sql
-- Adding a column with a DEFAULT in PostgreSQL 11+ is instant:
ALTER TABLE orders ADD COLUMN tracking TEXT DEFAULT NULL;
-- No table rewrite, instant metadata change, ACCESS EXCLUSIVE for milliseconds

-- But always use lock_timeout to be safe:
SET lock_timeout = '3s';
ALTER TABLE orders ADD COLUMN tracking TEXT DEFAULT NULL;
RESET lock_timeout;

-- Renaming a column (instant, metadata only):
SET lock_timeout = '3s';
ALTER TABLE orders RENAME COLUMN old_name TO new_name;
RESET lock_timeout;

-- Adding/removing NOT NULL constraint (PostgreSQL 12+ can be fast):
-- Step 1: Add constraint as NOT VALID (no full table scan)
ALTER TABLE orders ADD CONSTRAINT orders_status_nn
    CHECK (status IS NOT NULL) NOT VALID;
-- Step 2: Validate in background (SHARE UPDATE EXCLUSIVE, doesn't block writes)
ALTER TABLE orders VALIDATE CONSTRAINT orders_status_nn;
-- Step 3: Set actual NOT NULL (fast, since constraint is validated)
ALTER TABLE orders ALTER COLUMN status SET NOT NULL;
ALTER TABLE orders DROP CONSTRAINT orders_status_nn;
```

## Setting Up Alerting

```sql
-- Create a monitoring function for your alerting system
CREATE OR REPLACE FUNCTION check_lock_health()
RETURNS TABLE (
    metric TEXT,
    value BIGINT,
    status TEXT
) AS $$
BEGIN
    -- Check for long lock waits (> 30 seconds)
    RETURN QUERY
    SELECT
        'long_lock_waits'::TEXT,
        count(*)::BIGINT,
        CASE WHEN count(*) > 0 THEN 'WARNING' ELSE 'OK' END
    FROM pg_stat_activity
    WHERE wait_event_type = 'Lock'
      AND now() - query_start > INTERVAL '30 seconds';

    -- Check for idle-in-transaction (> 5 minutes)
    RETURN QUERY
    SELECT
        'idle_in_transaction'::TEXT,
        count(*)::BIGINT,
        CASE WHEN count(*) > 0 THEN 'CRITICAL' ELSE 'OK' END
    FROM pg_stat_activity
    WHERE state = 'idle in transaction'
      AND now() - xact_start > INTERVAL '5 minutes';

    -- Check table bloat (dead tuple ratio > 20%)
    RETURN QUERY
    SELECT
        'bloated_tables'::TEXT,
        count(*)::BIGINT,
        CASE WHEN count(*) > 3 THEN 'WARNING' ELSE 'OK' END
    FROM pg_stat_user_tables
    WHERE n_dead_tup > 10000
      AND n_dead_tup::float / GREATEST(n_live_tup, 1) > 0.2;

    -- Check XID age
    RETURN QUERY
    SELECT
        'xid_wraparound_risk'::TEXT,
        max(age(datfrozenxid))::BIGINT,
        CASE WHEN max(age(datfrozenxid)) > 500000000 THEN 'CRITICAL'
             WHEN max(age(datfrozenxid)) > 200000000 THEN 'WARNING'
             ELSE 'OK' END
    FROM pg_database;
END;
$$ LANGUAGE plpgsql;

-- Usage:
SELECT * FROM check_lock_health();
```



# Chapter 10: PostgreSQL vs. MySQL/InnoDB — Lock Comparison & Quick Reference

## Architectural Differences

| Feature | PostgreSQL | MySQL/InnoDB |
|---------|-----------|--------------|
| **MVCC implementation** | Tuple versioning in heap (xmin/xmax) | Undo log (rollback segment) |
| **Dead tuple cleanup** | VACUUM (background autovacuum) | Purge thread (automatic) |
| **Default isolation** | READ COMMITTED | REPEATABLE READ |
| **Dirty reads possible?** | Never (even at READ UNCOMMITTED) | Yes, at READ UNCOMMITTED |
| **Row lock storage** | In tuple header (t_xmax) | Lock manager (memory hash table) |
| **Lock escalation** | Never | Never |
| **Gap locks** | No (uses predicate locks at SERIALIZABLE only) | Yes (next-key locks at RR/S) |
| **SERIALIZABLE implementation** | SSI (readers don't block writers) | Gap locks + S locks (readers block writers) |
| **Advisory locks** | `pg_advisory_lock()` (rich API) | `GET_LOCK()` (basic) |
| **Table lock modes** | 8 modes (fine-grained) | Basic (READ/WRITE) |
| **Concurrent index creation** | `CREATE INDEX CONCURRENTLY` | None (InnoDB online DDL is partial) |

## MySQL/InnoDB Gap Locks and Next-Key Locks

MySQL/InnoDB uses **gap locks** and **next-key locks** at REPEATABLE READ and SERIALIZABLE. PostgreSQL does NOT use gap locks — this is a fundamental difference.

### What Gap Locks Are

```
InnoDB index with values: 10, 20, 30

Gaps:  (-∞, 10)  (10, 20)  (20, 30)  (30, +∞)

Next-key lock = record lock + gap lock on the gap BEFORE the record
  → next-key lock on 20 = gap (10, 20) + record 20 = range (10, 20]

Example:
  SELECT * FROM t WHERE id = 20 FOR UPDATE;
  InnoDB locks: next-key lock on (10, 20]
  Prevents INSERT of id = 11, 12, ..., 19, 20

PostgreSQL equivalent:
  SELECT * FROM t WHERE id = 20 FOR UPDATE;
  Locks: ONLY the specific row where id = 20
  Does NOT prevent any inserts!
```

### MySQL Gap Lock Problem: Wide Range Locks

```sql
-- MySQL at REPEATABLE READ:
SELECT * FROM orders WHERE amount BETWEEN 100 AND 200 FOR UPDATE;
-- Lock acquired: next-key locks on ALL index records in [100, 200] range
-- PLUS gap locks on the gaps between them
-- This blocks INSERTs of any row with amount between 100 and 200!

-- PostgreSQL equivalent:
SELECT * FROM orders WHERE amount BETWEEN 100 AND 200 FOR UPDATE;
-- Locks: only the EXISTING rows in the range
-- Does NOT block inserts into the range
-- This gives PostgreSQL better INSERT concurrency
```

### Why PostgreSQL Doesn't Need Gap Locks

PostgreSQL prevents phantoms (at REPEATABLE READ) using MVCC snapshots. A re-executed query always sees the same snapshot, so newly inserted rows are invisible regardless. There's no need to physically block inserts.

At SERIALIZABLE, PostgreSQL tracks predicates (SIReadLock) to detect read-write conflicts. If a conflict would cause an anomaly, the transaction is aborted. This gives the same correctness as gap locks but with much better concurrency (readers never block writers).

## Key Behavioral Differences

### Difference 1: Implicit Locking on SELECT

```sql
-- PostgreSQL at any isolation level:
SELECT * FROM orders WHERE id = 1;
-- Acquires: ACCESS SHARE (table level) — compatible with everything except DROP/ALTER
-- NO row locks acquired
-- Writers can modify this row concurrently

-- MySQL at SERIALIZABLE:
SELECT * FROM orders WHERE id = 1;
-- Implicitly becomes: SELECT * FROM orders WHERE id = 1 FOR SHARE (LOCK IN SHARE MODE)
-- Acquires: shared next-key lock on the row
-- Writers BLOCK until this transaction commits!
```

### Difference 2: Conflict Detection at REPEATABLE READ

```sql
-- PostgreSQL REPEATABLE READ detects write conflicts:
-- T1: UPDATE accounts SET balance = 500 WHERE id = 1;
-- T2: UPDATE accounts SET balance = 800 WHERE id = 1;
-- T2 gets: ERROR: could not serialize access due to concurrent update

-- MySQL REPEATABLE READ does NOT detect this:
-- T1: UPDATE accounts SET balance = 500 WHERE id = 1;
-- T2: blocks until T1 commits, then applies its update
-- No error — T2 overwrites T1. Lost update happens silently!
-- MySQL relies on gap locks for phantom prevention, not conflict detection.
```

### Difference 3: Consistent Non-Locking Read Point

```sql
-- PostgreSQL REPEATABLE READ: snapshot at first statement
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ;
-- (no snapshot yet)
SELECT * FROM orders;  -- snapshot created HERE
-- All subsequent SELECTs see this snapshot

-- MySQL REPEATABLE READ: snapshot at first consistent read
START TRANSACTION;
-- (no snapshot yet)
SELECT * FROM orders;  -- snapshot created HERE (same as PG)
-- But LOCK IN SHARE MODE / FOR UPDATE bypass snapshot
```

## Quick Reference: PostgreSQL Lock Commands

### Row Locks

```sql
-- Strongest to weakest:
SELECT * FROM t WHERE id = 1 FOR UPDATE;           -- blocks all other row locks
SELECT * FROM t WHERE id = 1 FOR NO KEY UPDATE;     -- allows FK checks
SELECT * FROM t WHERE id = 1 FOR SHARE;             -- shared read lock
SELECT * FROM t WHERE id = 1 FOR KEY SHARE;         -- lightest, FK check only

-- Modifiers:
... FOR UPDATE NOWAIT;                               -- fail immediately if locked
... FOR UPDATE SKIP LOCKED;                          -- skip locked rows
... FOR UPDATE OF table_alias;                       -- lock specific table only
```

### Table Locks

```sql
-- Explicit (inside transaction):
LOCK TABLE t IN ACCESS SHARE MODE;
LOCK TABLE t IN ROW SHARE MODE;
LOCK TABLE t IN ROW EXCLUSIVE MODE;
LOCK TABLE t IN SHARE UPDATE EXCLUSIVE MODE;
LOCK TABLE t IN SHARE MODE;
LOCK TABLE t IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE t IN EXCLUSIVE MODE;
LOCK TABLE t IN ACCESS EXCLUSIVE MODE;

-- Default LOCK TABLE mode is ACCESS EXCLUSIVE (strongest)
LOCK TABLE t;  -- same as: LOCK TABLE t IN ACCESS EXCLUSIVE MODE;
```

### Advisory Locks

```sql
-- Session-level (must explicitly unlock, or disconnect)
SELECT pg_advisory_lock(bigint_key);
SELECT pg_advisory_lock(int_key1, int_key2);
SELECT pg_try_advisory_lock(bigint_key);          -- non-blocking
SELECT pg_advisory_unlock(bigint_key);
SELECT pg_advisory_unlock_all();                  -- release all session locks

-- Transaction-level (auto-released on COMMIT/ROLLBACK)
SELECT pg_advisory_xact_lock(bigint_key);
SELECT pg_try_advisory_xact_lock(bigint_key);     -- non-blocking

-- Shared advisory locks (multiple holders allowed)
SELECT pg_advisory_lock_shared(bigint_key);
SELECT pg_advisory_xact_lock_shared(bigint_key);
```

### System Diagnostics

```sql
-- Current locks
SELECT * FROM pg_locks;

-- Current sessions
SELECT * FROM pg_stat_activity;

-- Who's blocking whom
SELECT pg_blocking_pids(pid) FROM pg_stat_activity WHERE pid = :target;

-- Cancel a query (graceful)
SELECT pg_cancel_backend(:pid);

-- Kill a session (forceful)
SELECT pg_terminate_backend(:pid);

-- Lock settings
SHOW lock_timeout;
SHOW deadlock_timeout;
SHOW statement_timeout;
SHOW idle_in_transaction_session_timeout;
SHOW max_locks_per_transaction;
```

## Summary: The Lock Decision Flowchart

```
START: Do you need to prevent concurrent modification?
│
├── NO → Use plain SELECT (MVCC snapshot handles reads)
│
├── YES → Is the operation a simple increment/decrement?
│   │
│   ├── YES → Use atomic UPDATE: SET val = val + 1
│   │         (no explicit lock needed)
│   │
│   └── NO → Is conflict frequency high?
│       │
│       ├── YES → Use pessimistic locking:
│       │         SELECT ... FOR UPDATE
│       │         Short transaction, consistent ordering
│       │
│       └── NO → Use optimistic locking:
│                 @Version column + retry logic
│
│   Need to prevent write skew?
│   ├── YES → SERIALIZABLE isolation + retry logic
│   └── NO  → READ COMMITTED + explicit locks when needed
│
│   Need coordinated access across processes/services?
│   ├── Same database → Advisory locks
│   └── Multiple systems → Distributed locks (Redis/ZK + fencing tokens)
│
│   Doing DDL on production tables?
│   └── ALWAYS SET lock_timeout FIRST
│       Use CONCURRENTLY variants when available
│       Schedule during low-traffic windows for ACCESS EXCLUSIVE ops
```

---

*End of tutorial. This guide covered database locking from fundamentals through advanced PostgreSQL internals. For further learning, read the [PostgreSQL documentation on Explicit Locking](https://www.postgresql.org/docs/current/explicit-locking.html) and the SSI paper by Cahill, Röhm, and Fekete (2008).*


