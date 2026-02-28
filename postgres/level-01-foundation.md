# Level 1 — The Foundation

> Your complete go-to reference for getting started with PostgreSQL.  
> By the end of this guide you'll be comfortable creating databases, designing tables with proper data types and constraints, and writing all fundamental SQL queries.

---

## Table of Contents

1. [What is PostgreSQL?](#1-what-is-postgresql)
2. [Installation & Setup](#2-installation--setup)
3. [The `psql` Command-Line Client](#3-the-psql-command-line-client)
4. [Databases — Create, Connect, Drop](#4-databases--create-connect-drop)
5. [Data Types](#5-data-types)
6. [Tables — Create, Alter, Drop](#6-tables--create-alter-drop)
7. [Constraints](#7-constraints)
8. [INSERT — Adding Data](#8-insert--adding-data)
9. [SELECT — Reading Data](#9-select--reading-data)
10. [WHERE — Filtering Rows](#10-where--filtering-rows)
11. [UPDATE & DELETE — Modifying Data](#11-update--delete--modifying-data)
12. [UPSERT — INSERT ON CONFLICT](#12-upsert--insert-on-conflict)
13. [Sorting, Limiting & Pagination](#13-sorting-limiting--pagination)
14. [Aggregate Functions & Grouping](#14-aggregate-functions--grouping)
15. [DISTINCT & DISTINCT ON](#15-distinct--distinct-on)
16. [Type Casting](#16-type-casting)
17. [Practice Exercises](#17-practice-exercises)

---

## 1. What is PostgreSQL?

PostgreSQL (often called **Postgres**) is an **Object-Relational Database Management System (ORDBMS)**. It's open source, incredibly standards-compliant, and arguably the most feature-rich relational database in existence.

### Why does it matter to you as a backend dev?

| Feature | What it means |
|---|---|
| **ACID Compliant** | Every transaction is **Atomic** (all-or-nothing), **Consistent** (valid state to valid state), **Isolated** (concurrent txns don't interfere), **Durable** (committed = on disk). Your data is safe. |
| **MVCC** | Multi-Version Concurrency Control — readers never block writers, writers never block readers. This is *the* reason Postgres handles high concurrency so well. (Deep dive in Level 6) |
| **Extensible** | You can create custom data types, operators, index types, and even write functions in Python, JavaScript, or C. |
| **SQL Standards** | Postgres implements more of the SQL standard than any other database. What you learn here is highly portable. |

### Postgres vs Others (Quick Summary)

| | PostgreSQL | MySQL | SQLite |
|---|---|---|---|
| Concurrency | MVCC (excellent) | InnoDB uses MVCC; MyISAM uses table locks | File-level locking |
| JSON support | `JSONB` with indexing | JSON (limited indexing) | JSON functions |
| Extensibility | Extensions, custom types | Limited | Limited |
| Best for | Everything from startups to enterprise | Web apps (LAMP stack) | Embedded/mobile/small apps |

---

## 2. Installation & Setup

### Option A: Docker (Recommended for Learning)

```bash
# Pull and run Postgres 17
docker run --name pg-learn \
  -e POSTGRES_USER=admin \
  -e POSTGRES_PASSWORD=secret \
  -e POSTGRES_DB=playground \
  -p 5432:5432 \
  -d postgres:17

# Connect with psql (from the container)
docker exec -it pg-learn psql -U admin -d playground
```

### Option B: Native Install

- **Windows**: Download from [postgresql.org/download/windows](https://www.postgresql.org/download/windows/) — includes pgAdmin GUI
- **macOS**: `brew install postgresql@17` then `brew services start postgresql@17`
- **Linux (Ubuntu/Debian)**: 
```bash
sudo apt install postgresql postgresql-contrib
sudo systemctl start postgresql
sudo -u postgres psql   # connect as superuser
```

### Verify it works

```sql
SELECT version();
```
```
                          version
------------------------------------------------------------
 PostgreSQL 17.0 on x86_64-pc-linux-gnu, compiled by gcc...
```

---

## 3. The `psql` Command-Line Client

`psql` is the official interactive terminal for Postgres. It's incredibly powerful — learn these commands and you won't need a GUI.

### Essential Meta-Commands

Meta-commands start with `\` and are psql-specific (not SQL).

| Command | What it does | Example |
|---|---|---|
| `\l` | List all databases | `\l` |
| `\c dbname` | Connect to a database | `\c playground` |
| `\dt` | List all tables in current schema | `\dt` |
| `\dt+` | List tables with sizes | `\dt+` |
| `\d tablename` | Describe a table (columns, types, constraints) | `\d users` |
| `\d+ tablename` | Detailed table description (storage, stats) | `\d+ users` |
| `\di` | List all indexes | `\di` |
| `\dn` | List all schemas | `\dn` |
| `\df` | List all functions | `\df` |
| `\du` | List all roles/users | `\du` |
| `\x` | Toggle expanded display (vertical output) | `\x` then run a query |
| `\timing` | Toggle query timing | `\timing` |
| `\e` | Open last query in `$EDITOR` | `\e` |
| `\i file.sql` | Execute SQL from a file | `\i setup.sql` |
| `\q` | Quit psql | `\q` |
| `\?` | Help for meta-commands | `\?` |
| `\h COMMAND` | SQL syntax help | `\h CREATE TABLE` |

### Pro Tips for psql

```sql
-- Turn on timing to see how long queries take
\timing

-- Turn on expanded display for wide tables
\x auto

-- Show the SQL behind a meta-command (educational!)
-- Start psql with -E flag
psql -U admin -d playground -E
-- Now every \dt, \d etc. will show you the actual SQL it runs!
```

> **💡 Tip:** `\x auto` is better than `\x` — it automatically switches to vertical mode only when the output is too wide for your terminal.

---

## 4. Databases — Create, Connect, Drop

A Postgres **cluster** (server instance) can host multiple **databases**. Each database is a fully isolated namespace — tables in one database can't reference tables in another.

```sql
-- Create a new database
CREATE DATABASE bookstore;

-- Create with specific settings
CREATE DATABASE bookstore
    OWNER = admin
    ENCODING = 'UTF8'
    LC_COLLATE = 'en_US.UTF-8'
    LC_CTYPE = 'en_US.UTF-8';

-- Connect to it
\c bookstore

-- See current database
SELECT current_database();

-- Rename a database (no one can be connected to it)
ALTER DATABASE bookstore RENAME TO my_bookstore;

-- Drop a database (IRREVERSIBLE! use with caution)
DROP DATABASE my_bookstore;

-- Drop only if it exists (avoids error)
DROP DATABASE IF EXISTS my_bookstore;
```

> **⚠️ Warning:** You cannot `DROP` a database you're currently connected to. Connect to a different database first (e.g., `\c postgres`).

---

## 5. Data Types

Choosing the right data type is critical — it affects storage, performance, and data integrity. Here are the ones you'll use 95% of the time:

### Numeric Types

| Type | Size | Range | When to use |
|---|---|---|---|
| `SMALLINT` | 2 bytes | -32,768 to 32,767 | Counters, status codes |
| `INTEGER` (or `INT`) | 4 bytes | -2.1B to 2.1B | Default choice for whole numbers |
| `BIGINT` | 8 bytes | ±9.2 quintillion | IDs in high-volume tables, timestamps-as-ints |
| `NUMERIC(p,s)` | variable | up to 131,072 digits | Money, precision-critical math |
| `REAL` | 4 bytes | 6 decimal digits | Approximate calcs (avoid for money!) |
| `DOUBLE PRECISION` | 8 bytes | 15 decimal digits | Scientific data |

```sql
-- NEVER use REAL or DOUBLE PRECISION for money!
-- Floating point is approximate:
SELECT 0.1::REAL + 0.2::REAL;        -- 0.30000001192092896 ❌
SELECT 0.1::NUMERIC + 0.2::NUMERIC;  -- 0.3 ✅
```

### Auto-Incrementing IDs

| Type | Underlying | Notes |
|---|---|---|
| `SERIAL` | `INTEGER` + sequence | Legacy, but widely used |
| `BIGSERIAL` | `BIGINT` + sequence | For high-volume tables |
| `GENERATED ALWAYS AS IDENTITY` | Standards-compliant | **Preferred in modern Postgres** |

```sql
-- Legacy style (still common)
CREATE TABLE users (
    id SERIAL PRIMARY KEY
);

-- Modern style (preferred)
CREATE TABLE users (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY
);

-- BIGINT identity for high-volume tables
CREATE TABLE events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY
);
```

**Why prefer `GENERATED ALWAYS AS IDENTITY`?**
- SQL standard compliant (portable)
- Can't accidentally insert manual values (unless you use `OVERRIDING SYSTEM VALUE`)
- The sequence is tightly coupled to the column (dropped with the column)

### String Types

| Type | Behavior | When to use |
|---|---|---|
| `TEXT` | Unlimited length, variable storage | **Default choice** for all strings |
| `VARCHAR(n)` | Variable length, max `n` chars | When you genuinely need a max length |
| `CHAR(n)` | Fixed length, space-padded | **Almost never** — wastes space |

```sql
-- There is NO performance difference between TEXT and VARCHAR in Postgres!
-- TEXT is simpler and preferred:
CREATE TABLE articles (
    title TEXT NOT NULL,
    slug VARCHAR(200) NOT NULL  -- only if you truly need to cap at 200
);
```

> **💡 Key insight:** In Postgres, `TEXT`, `VARCHAR`, and `VARCHAR(n)` all use the same internal storage mechanism (varlena). The `(n)` in `VARCHAR(n)` is just a constraint check, not a storage optimization. Use `TEXT` unless you have a business reason for a maximum length.

### Boolean

```sql
-- Accepts: TRUE, FALSE, NULL
-- Also accepts: 't', 'f', 'yes', 'no', '1', '0', 'on', 'off'
CREATE TABLE features (
    name TEXT NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT FALSE
);

INSERT INTO features (name, is_enabled) VALUES ('dark_mode', TRUE);
```

### Date & Time Types

| Type | Stores | Example |
|---|---|---|
| `DATE` | Date only | `'2026-02-28'` |
| `TIME` | Time only (no timezone) | `'14:30:00'` |
| `TIMESTAMP` | Date + time, **no timezone** | `'2026-02-28 14:30:00'` |
| `TIMESTAMPTZ` | Date + time, **with timezone** | `'2026-02-28 14:30:00+05:30'` |
| `INTERVAL` | Duration | `'3 days 2 hours'` |

```sql
-- ⚠️ ALWAYS use TIMESTAMPTZ, not TIMESTAMP

-- TIMESTAMP without timezone is DANGEROUS:
-- It stores literally what you give it, with no timezone context.
-- If your app server is in UTC and your DB session is in IST,
-- you get silent data corruption.

-- TIMESTAMPTZ: Postgres converts to UTC for storage, converts
-- back to session timezone for display. Always correct.

-- Example:
SET timezone = 'Asia/Kolkata';

CREATE TABLE logs (
    id SERIAL PRIMARY KEY,
    message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO logs (message) VALUES ('Server started');
SELECT * FROM logs;
-- created_at shows: 2026-02-28 16:08:35+05:30

SET timezone = 'UTC';
SELECT * FROM logs;
-- created_at shows: 2026-02-28 10:38:35+00 (same instant, different display!)
```

### Useful Date/Time Functions

```sql
-- Current timestamp
SELECT NOW();                      -- 2026-02-28 16:08:35.123456+05:30
SELECT CURRENT_TIMESTAMP;          -- same as NOW()
SELECT CURRENT_DATE;               -- 2026-02-28
SELECT CURRENT_TIME;               -- 16:08:35.123456+05:30

-- Extract parts
SELECT EXTRACT(YEAR FROM NOW());   -- 2026
SELECT EXTRACT(DOW FROM NOW());    -- 0=Sunday, 6=Saturday
SELECT DATE_PART('hour', NOW());   -- 16

-- Date math
SELECT NOW() + INTERVAL '7 days';          -- one week from now
SELECT NOW() - INTERVAL '3 hours';         -- 3 hours ago
SELECT AGE(NOW(), '2000-01-01'::DATE);     -- 26 years 1 mon 27 days...
SELECT '2026-03-01'::DATE - '2026-02-28'::DATE;  -- 1 (integer days)

-- Truncate to start of period
SELECT DATE_TRUNC('month', NOW());   -- 2026-02-01 00:00:00+05:30
SELECT DATE_TRUNC('hour', NOW());    -- 2026-02-28 16:00:00+05:30
```

### UUID

```sql
-- Enable the extension (if not already)
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Random UUID (v4)
SELECT gen_random_uuid();   -- a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11

-- Use as primary key
CREATE TABLE orders (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    customer_name TEXT NOT NULL,
    total NUMERIC(10, 2) NOT NULL
);

-- gen_random_uuid() is built-in since Postgres 13, no extension needed!
```

### JSONB

```sql
-- JSONB = binary JSON. Supports indexing, fast lookups.
-- JSON = text JSON. Just stored as-is. Almost never use this.
CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    metadata JSONB DEFAULT '{}'
);

INSERT INTO products (name, metadata) VALUES
    ('Laptop', '{"brand": "Dell", "ram_gb": 16, "tags": ["electronics", "work"]}');

-- Access fields
SELECT metadata->>'brand' FROM products;         -- 'Dell' (as text)
SELECT metadata->'ram_gb' FROM products;          -- 16 (as jsonb)
SELECT metadata->'tags'->0 FROM products;         -- "electronics"
```

> We'll dive deep into JSONB operators and indexing in Level 8. For now, just know it exists and is incredibly powerful.

---

## 6. Tables — Create, Alter, Drop

### CREATE TABLE

```sql
CREATE TABLE employees (
    id          INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    first_name  TEXT NOT NULL,
    last_name   TEXT NOT NULL,
    email       TEXT UNIQUE NOT NULL,
    department  TEXT DEFAULT 'general',
    salary      NUMERIC(10, 2) CHECK (salary > 0),
    is_active   BOOLEAN DEFAULT TRUE,
    hired_at    DATE NOT NULL DEFAULT CURRENT_DATE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);
```

**What's happening here:**
- `GENERATED ALWAYS AS IDENTITY` — auto-incrementing ID, SQL standard
- `NOT NULL` — this column must always have a value
- `UNIQUE` — no two rows can have the same email
- `DEFAULT` — value used when not explicitly provided
- `CHECK` — a validation rule enforced by the database

### ALTER TABLE

```sql
-- Add a column
ALTER TABLE employees ADD COLUMN phone TEXT;

-- Add a column with a default (backfills existing rows)
ALTER TABLE employees ADD COLUMN country TEXT DEFAULT 'IN';

-- Rename a column
ALTER TABLE employees RENAME COLUMN phone TO phone_number;

-- Change a column's data type
ALTER TABLE employees ALTER COLUMN phone_number TYPE VARCHAR(15);

-- Set / remove a default
ALTER TABLE employees ALTER COLUMN department SET DEFAULT 'engineering';
ALTER TABLE employees ALTER COLUMN department DROP DEFAULT;

-- Add / remove NOT NULL
ALTER TABLE employees ALTER COLUMN phone_number SET NOT NULL;
ALTER TABLE employees ALTER COLUMN phone_number DROP NOT NULL;

-- Add a constraint
ALTER TABLE employees ADD CONSTRAINT salary_positive CHECK (salary > 0);

-- Drop a constraint
ALTER TABLE employees DROP CONSTRAINT salary_positive;

-- Rename the table
ALTER TABLE employees RENAME TO staff;
```

### DROP TABLE

```sql
-- Drop a table (error if it doesn't exist)
DROP TABLE employees;

-- Drop only if it exists
DROP TABLE IF EXISTS employees;

-- Drop and also drop everything that depends on it (foreign keys, views, etc.)
DROP TABLE IF EXISTS employees CASCADE;
```

> **⚠️ `CASCADE` is dangerous** — it silently drops dependent objects. Always check what depends on a table first with:
> ```sql
> SELECT * FROM pg_depend WHERE refobjid = 'employees'::regclass;
> ```

---

## 7. Constraints

Constraints are **rules enforced by the database engine** that protect your data integrity. This is one of the most important concepts in database design.

### PRIMARY KEY

A primary key uniquely identifies every row. It's automatically `UNIQUE` and `NOT NULL`. An index is automatically created on it.

```sql
-- Single column PK
CREATE TABLE users (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username TEXT NOT NULL
);

-- Composite PK (multiple columns together form the key)
CREATE TABLE enrollments (
    student_id INTEGER,
    course_id INTEGER,
    enrolled_at DATE DEFAULT CURRENT_DATE,
    PRIMARY KEY (student_id, course_id)
);
-- A student can enroll in many courses, a course can have many students,
-- but a student can only enroll in the same course ONCE.
```

### NOT NULL

```sql
-- Column must always have a value — NULLs are rejected
CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,          -- ✅ required
    description TEXT             -- ❌ nullable (may or may not have data)
);

INSERT INTO products (description) VALUES ('A nice product');
-- ERROR: null value in column "name" violates not-null constraint
```

### UNIQUE

```sql
-- No two rows can have the same value in this column
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    username TEXT UNIQUE NOT NULL
);

-- Multi-column unique (combination must be unique)
CREATE TABLE permissions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    resource TEXT NOT NULL,
    UNIQUE (user_id, resource)
);
-- Same user can't have the same resource twice,
-- but different users can have the same resource.
```

> **💡 Gotcha:** `NULL` values are treated as distinct in `UNIQUE` constraints. So you can have multiple rows with `NULL` in a unique column! Use `NOT NULL` if you want to prevent this.

### CHECK

```sql
CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    price NUMERIC(10, 2) CHECK (price > 0),
    discount NUMERIC(5, 2) DEFAULT 0 CHECK (discount >= 0 AND discount <= 100),
    stock INTEGER DEFAULT 0 CHECK (stock >= 0),
    
    -- Multi-column check
    CONSTRAINT valid_dates CHECK (end_date > start_date)
);

INSERT INTO products (name, price, stock) VALUES ('Widget', -5.00, 10);
-- ERROR: new row violates check constraint "products_price_check"
```

### DEFAULT

```sql
CREATE TABLE activities (
    id SERIAL PRIMARY KEY,
    action TEXT NOT NULL,
    status TEXT DEFAULT 'pending',
    priority INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    metadata JSONB DEFAULT '{}'::JSONB
);

-- When inserting, omitted columns get their defaults:
INSERT INTO activities (action) VALUES ('deploy');
-- Result: status='pending', priority=0, created_at=<now>, metadata={}
```

### FOREIGN KEY (Preview)

Foreign keys enforce relationships between tables. We'll cover this in depth in Level 2, but here's a preview:

```sql
CREATE TABLE departments (
    id SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

CREATE TABLE employees (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    department_id INTEGER REFERENCES departments(id) ON DELETE SET NULL
);

-- You can't insert an employee with a department_id that doesn't exist:
INSERT INTO employees (name, department_id) VALUES ('Alice', 999);
-- ERROR: insert or update on table "employees" violates foreign key constraint
```

---

## 8. INSERT — Adding Data

### Single Row Insert

```sql
INSERT INTO employees (first_name, last_name, email, salary)
VALUES ('Siddharth', 'Saurav', 'sid@example.com', 85000.00);
```

### Multi-Row Insert

```sql
-- Much faster than individual INSERTs (single transaction, single parse)
INSERT INTO employees (first_name, last_name, email, salary)
VALUES
    ('Alice', 'Johnson', 'alice@example.com', 92000.00),
    ('Bob', 'Williams', 'bob@example.com', 78000.00),
    ('Carol', 'Davis', 'carol@example.com', 105000.00);
```

### INSERT with RETURNING

This is a **Postgres superpower** — get back the data that was just inserted without a separate `SELECT`:

```sql
INSERT INTO employees (first_name, last_name, email, salary)
VALUES ('Dave', 'Miller', 'dave@example.com', 67000.00)
RETURNING id, email, created_at;

-- Result:
--  id |       email       |          created_at
-- ----+-------------------+-------------------------------
--   5 | dave@example.com  | 2026-02-28 16:08:35.123+05:30
```

> **💡 Backend tip:** `RETURNING *` is essential. In your backend code, after an `INSERT`, you often need the created record (with its generated id, defaults, etc.). Instead of doing `INSERT` then `SELECT`, just use `RETURNING`.

### INSERT from a SELECT

```sql
-- Copy data from one table to another
INSERT INTO employees_archive (first_name, last_name, email, salary)
SELECT first_name, last_name, email, salary
FROM employees
WHERE is_active = FALSE;
```

---

## 9. SELECT — Reading Data

### Basics

```sql
-- All columns, all rows
SELECT * FROM employees;

-- Specific columns
SELECT first_name, last_name, salary FROM employees;

-- Aliasing columns
SELECT
    first_name AS "First Name",
    last_name AS "Last Name",
    salary * 12 AS annual_salary
FROM employees;

-- Expressions
SELECT
    first_name || ' ' || last_name AS full_name,   -- string concat
    salary * 1.10 AS salary_after_raise,            -- math
    UPPER(email) AS email_upper,                    -- function
    LENGTH(first_name) AS name_length               -- function
FROM employees;
```

### Column Aliases with AS

```sql
-- AS is optional but recommended for readability:
SELECT first_name AS name FROM employees;
SELECT first_name name FROM employees;    -- same thing, but less clear

-- Use double quotes for aliases with spaces or uppercase:
SELECT salary * 12 AS "Annual Salary" FROM employees;
```

---

## 10. WHERE — Filtering Rows

### Comparison Operators

```sql
-- Equality
SELECT * FROM employees WHERE department = 'engineering';

-- Not equal (both work)
SELECT * FROM employees WHERE department != 'sales';
SELECT * FROM employees WHERE department <> 'sales';

-- Numeric comparisons
SELECT * FROM employees WHERE salary > 80000;
SELECT * FROM employees WHERE salary >= 80000;
SELECT * FROM employees WHERE salary < 100000;
SELECT * FROM employees WHERE salary <= 100000;
```

### Logical Operators

```sql
-- AND: both conditions must be true
SELECT * FROM employees
WHERE department = 'engineering' AND salary > 80000;

-- OR: at least one condition must be true
SELECT * FROM employees
WHERE department = 'engineering' OR department = 'design';

-- NOT: negate a condition
SELECT * FROM employees
WHERE NOT department = 'sales';

-- Combining with parentheses (IMPORTANT for precedence!)
SELECT * FROM employees
WHERE (department = 'engineering' OR department = 'design')
  AND salary > 80000;
-- Without parentheses, AND binds tighter than OR, giving wrong results.
```

### BETWEEN

```sql
-- Inclusive on both ends
SELECT * FROM employees
WHERE salary BETWEEN 70000 AND 100000;
-- Same as: WHERE salary >= 70000 AND salary <= 100000

-- Works with dates too
SELECT * FROM employees
WHERE hired_at BETWEEN '2025-01-01' AND '2025-12-31';
```

### IN

```sql
-- Match against a list of values (much cleaner than multiple ORs)
SELECT * FROM employees
WHERE department IN ('engineering', 'design', 'product');

-- NOT IN
SELECT * FROM employees
WHERE department NOT IN ('sales', 'marketing');
```

> **⚠️ Gotcha:** `NOT IN` with `NULL` values returns unexpected results! If the list contains even one `NULL`, `NOT IN` returns no rows. Use `NOT EXISTS` instead (covered in Level 2).

### LIKE & ILIKE — Pattern Matching

```sql
-- LIKE: case-sensitive
SELECT * FROM employees WHERE email LIKE '%@gmail.com';

-- Pattern symbols:
--   %  = any number of characters (including zero)
--   _  = exactly one character

SELECT * FROM employees WHERE first_name LIKE 'A%';        -- starts with A
SELECT * FROM employees WHERE last_name LIKE '%son';        -- ends with son
SELECT * FROM employees WHERE email LIKE '%@%.com';         -- contains @ and .com
SELECT * FROM employees WHERE first_name LIKE '____';       -- exactly 4 characters

-- ILIKE: case-INSENSITIVE (Postgres extension, not standard SQL)
SELECT * FROM employees WHERE first_name ILIKE 'alice';     -- matches Alice, ALICE, alice
```

### IS NULL / IS NOT NULL

```sql
-- NULL is NOT a value, it's the ABSENCE of a value.
-- You CANNOT use = NULL or != NULL. They don't work!

-- ❌ WRONG (always evaluates to NULL, never TRUE or FALSE)
SELECT * FROM employees WHERE phone_number = NULL;

-- ✅ CORRECT
SELECT * FROM employees WHERE phone_number IS NULL;
SELECT * FROM employees WHERE phone_number IS NOT NULL;
```

> **💡 Critical concept:** `NULL` is not equal to anything, not even itself. `NULL = NULL` is `NULL` (not `TRUE`). This is called **three-valued logic** — every expression evaluates to `TRUE`, `FALSE`, or `NULL`.

---

## 11. UPDATE & DELETE — Modifying Data

### UPDATE

```sql
-- Update single column for matching rows
UPDATE employees
SET salary = 90000
WHERE email = 'alice@example.com';

-- Update multiple columns
UPDATE employees
SET salary = 95000,
    department = 'senior engineering',
    updated_at = NOW()
WHERE id = 2;

-- Update with expression
UPDATE employees
SET salary = salary * 1.10        -- 10% raise
WHERE department = 'engineering';

-- RETURNING: see what was updated
UPDATE employees
SET salary = salary * 1.05
WHERE department = 'design'
RETURNING id, first_name, salary;
```

> **⚠️ Without WHERE, UPDATE affects ALL rows!** Always double-check your WHERE clause. Consider wrapping risky updates in a transaction:
> ```sql
> BEGIN;
> UPDATE employees SET salary = 0;   -- oops, forgot WHERE!
> -- Check what happened:
> SELECT * FROM employees;
> ROLLBACK;  -- undo everything!
> ```

### DELETE

```sql
-- Delete specific rows
DELETE FROM employees
WHERE id = 5;

-- Delete with condition
DELETE FROM employees
WHERE is_active = FALSE AND hired_at < '2020-01-01';

-- Delete all rows (keeps the table structure)
DELETE FROM employees;

-- Faster way to delete ALL rows (resets table, can't rollback easily)
TRUNCATE TABLE employees;

-- TRUNCATE with identity reset
TRUNCATE TABLE employees RESTART IDENTITY;

-- RETURNING: see what was deleted
DELETE FROM employees
WHERE department = 'temp'
RETURNING *;
```

**DELETE vs TRUNCATE:**

| | `DELETE` | `TRUNCATE` |
|---|---|---|
| Speed | Slow (row by row) | Instant |
| WHERE clause | ✅ Yes | ❌ No (all rows) |
| Triggers fire | ✅ Yes | ❌ No |
| MVCC / rollback | ✅ Yes (creates dead tuples) | Partially (resets files) |
| Resets sequences | ❌ No | ✅ With `RESTART IDENTITY` |

---

## 12. UPSERT — INSERT ON CONFLICT

One of the most useful Postgres features for backend developers. "Insert this row, but if it conflicts with an existing row, update it instead."

```sql
-- Simple upsert: insert or update on email conflict
INSERT INTO employees (first_name, last_name, email, salary)
VALUES ('Alice', 'Johnson', 'alice@example.com', 95000)
ON CONFLICT (email)
DO UPDATE SET
    salary = EXCLUDED.salary,
    updated_at = NOW();

-- EXCLUDED refers to the row that WOULD HAVE been inserted.
-- So EXCLUDED.salary = 95000 (the new value we tried to insert).
```

### DO NOTHING (Skip on Conflict)

```sql
-- Just skip the insert if the row already exists
INSERT INTO employees (first_name, last_name, email, salary)
VALUES ('Bob', 'Williams', 'bob@example.com', 78000)
ON CONFLICT (email)
DO NOTHING;
```

### Upsert with Composite Key

```sql
-- Conflict on composite unique constraint
INSERT INTO enrollments (student_id, course_id, enrolled_at)
VALUES (1, 101, CURRENT_DATE)
ON CONFLICT (student_id, course_id)
DO UPDATE SET enrolled_at = EXCLUDED.enrolled_at;
```

### Upsert with WHERE (Conditional Update)

```sql
-- Only update if the new salary is higher
INSERT INTO employees (first_name, last_name, email, salary)
VALUES ('Alice', 'Johnson', 'alice@example.com', 95000)
ON CONFLICT (email)
DO UPDATE SET salary = EXCLUDED.salary
WHERE EXCLUDED.salary > employees.salary;
-- If new salary <= current salary, nothing happens.
```

> **💡 Backend tip:** `ON CONFLICT` is perfect for idempotent operations. API endpoints that might be called multiple times (retries, webhooks) should use upserts to avoid duplicate data.

---

## 13. Sorting, Limiting & Pagination

### ORDER BY

```sql
-- Ascending (default)
SELECT * FROM employees ORDER BY salary;
SELECT * FROM employees ORDER BY salary ASC;    -- explicit

-- Descending
SELECT * FROM employees ORDER BY salary DESC;

-- Multiple sort columns (tie-breaker)
SELECT * FROM employees
ORDER BY department ASC, salary DESC;
-- First sort by department alphabetically,
-- then within each department, show highest salary first.

-- NULLs: by default, NULLs sort LAST in ASC, FIRST in DESC.
-- Override with:
SELECT * FROM employees ORDER BY phone_number ASC NULLS FIRST;
SELECT * FROM employees ORDER BY phone_number DESC NULLS LAST;

-- Sort by column position (not recommended, but you'll see it)
SELECT first_name, salary FROM employees ORDER BY 2 DESC;  -- 2 = salary
```

### LIMIT & OFFSET

```sql
-- Get first 10 rows
SELECT * FROM employees ORDER BY id LIMIT 10;

-- Get rows 11-20 (skip first 10)
SELECT * FROM employees ORDER BY id LIMIT 10 OFFSET 10;

-- Page 3, 10 items per page
SELECT * FROM employees ORDER BY id LIMIT 10 OFFSET 20;
```

> **⚠️ Performance Warning:** `OFFSET` gets slower as the number grows! `OFFSET 1000000` means Postgres must scan and discard 1 million rows. For large datasets, use **keyset pagination** instead (covered in Level 5):
> ```sql
> -- Instead of OFFSET, remember the last ID and fetch after it:
> SELECT * FROM employees
> WHERE id > 1000       -- last seen id
> ORDER BY id
> LIMIT 10;
> ```

---

## 14. Aggregate Functions & Grouping

Aggregates compute a single value from a set of rows.

### Basic Aggregates

```sql
SELECT COUNT(*) FROM employees;                    -- total rows: 5
SELECT COUNT(phone_number) FROM employees;         -- non-NULL phones: 3
SELECT SUM(salary) FROM employees;                 -- total payroll: 427000.00
SELECT AVG(salary) FROM employees;                 -- average salary: 85400.00
SELECT MIN(salary) FROM employees;                 -- lowest salary: 67000.00
SELECT MAX(salary) FROM employees;                 -- highest salary: 105000.00
SELECT MIN(hired_at) AS earliest_hire FROM employees; -- earliest hire date
```

### GROUP BY

"Give me the aggregate **for each group**."

```sql
-- Average salary per department
SELECT department, AVG(salary) AS avg_salary
FROM employees
GROUP BY department;

-- Result:
--  department   | avg_salary
-- --------------+------------
--  engineering  | 88500.00
--  design       | 78000.00
--  product      | 105000.00

-- Count employees per department, sorted
SELECT department, COUNT(*) AS employee_count
FROM employees
GROUP BY department
ORDER BY employee_count DESC;

-- Multiple aggregates
SELECT
    department,
    COUNT(*) AS headcount,
    ROUND(AVG(salary), 2) AS avg_salary,
    MIN(salary) AS min_salary,
    MAX(salary) AS max_salary,
    SUM(salary) AS total_payroll
FROM employees
GROUP BY department;
```

> **💡 Rule:** Every column in the `SELECT` list must either be in `GROUP BY` or be wrapped in an aggregate function. Otherwise, Postgres doesn't know which value to pick for that column.

### HAVING — Filter After Grouping

`WHERE` filters rows **before** grouping. `HAVING` filters groups **after** aggregation.

```sql
-- Departments with more than 3 employees
SELECT department, COUNT(*) AS headcount
FROM employees
GROUP BY department
HAVING COUNT(*) > 3;

-- Departments where average salary exceeds 90k
SELECT department, AVG(salary) AS avg_salary
FROM employees
GROUP BY department
HAVING AVG(salary) > 90000;
```

### Execution Order

Understanding the SQL execution order prevents confusion:

```
1. FROM         → Which table(s)?
2. WHERE        → Filter individual rows
3. GROUP BY     → Group remaining rows
4. HAVING       → Filter groups
5. SELECT       → Choose columns / compute expressions
6. DISTINCT     → Remove duplicates
7. ORDER BY     → Sort results
8. LIMIT/OFFSET → Paginate
```

> **💡 This is why you can't use column aliases in WHERE:**
> ```sql
> SELECT salary * 12 AS annual FROM employees WHERE annual > 100000;
> -- ERROR! WHERE runs before SELECT, so "annual" doesn't exist yet.
> -- Fix: repeat the expression
> SELECT salary * 12 AS annual FROM employees WHERE salary * 12 > 100000;
> ```

---

## 15. DISTINCT & DISTINCT ON

### DISTINCT

```sql
-- Unique values in a column
SELECT DISTINCT department FROM employees;

-- Unique combinations
SELECT DISTINCT department, is_active FROM employees;
```

### DISTINCT ON (Postgres-specific!)

Returns the **first row** for each distinct value of the specified columns. Extremely useful for "latest record per group" patterns.

```sql
-- Get the most recent order for each customer
SELECT DISTINCT ON (customer_id)
    customer_id,
    order_id,
    total,
    created_at
FROM orders
ORDER BY customer_id, created_at DESC;

-- For each customer_id, it keeps only the row with the latest created_at.
```

> **💡 Backend gem:** `DISTINCT ON` is one of Postgres's most underrated features. It solves the "get the latest X for each Y" problem in a single, clean query — no window functions or subqueries needed.

**How it works:**
1. `ORDER BY customer_id, created_at DESC` — sort the data
2. `DISTINCT ON (customer_id)` — for each unique `customer_id`, keep only the first row (which has the latest `created_at` because of the `DESC` sort)

---

## 16. Type Casting

Convert values between types using `::` (Postgres syntax) or `CAST()` (SQL standard).

```sql
-- :: syntax (Postgres shorthand, preferred)
SELECT '42'::INTEGER;                 -- text → integer
SELECT 42::TEXT;                      -- integer → text
SELECT '3.14'::NUMERIC;              -- text → numeric
SELECT '2026-02-28'::DATE;           -- text → date
SELECT 'true'::BOOLEAN;              -- text → boolean
SELECT '{"a":1}'::JSONB;             -- text → jsonb

-- CAST() syntax (SQL standard)
SELECT CAST('42' AS INTEGER);
SELECT CAST('2026-02-28' AS DATE);

-- Practical examples
SELECT salary::TEXT || ' USD' AS formatted FROM employees;
SELECT EXTRACT(EPOCH FROM NOW())::INTEGER AS unix_timestamp;
SELECT ('2026-02-28'::DATE + 30)::TEXT AS thirty_days_later;
```

### Common Pitfalls

```sql
-- Integer division truncates!
SELECT 5 / 2;           -- Result: 2  (not 2.5!)
SELECT 5.0 / 2;         -- Result: 2.5 ✅
SELECT 5::NUMERIC / 2;  -- Result: 2.5 ✅

-- This trips up backend devs calculating percentages:
SELECT 45 / 100;         -- 0   ❌
SELECT 45::NUMERIC / 100; -- 0.45 ✅
```

---

## 17. Practice Exercises

Build a small **Library Management System** to practice everything from this level.

### Setup

```sql
CREATE DATABASE library;
\c library

-- Authors table
CREATE TABLE authors (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL,
    country TEXT DEFAULT 'Unknown',
    born_on DATE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Books table
CREATE TABLE books (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title TEXT NOT NULL,
    isbn TEXT UNIQUE NOT NULL,
    pages INTEGER CHECK (pages > 0),
    price NUMERIC(6, 2) CHECK (price >= 0),
    genre TEXT NOT NULL,
    published_at DATE,
    is_available BOOLEAN DEFAULT TRUE,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Seed data
INSERT INTO authors (name, country, born_on) VALUES
    ('Robert C. Martin', 'US', '1952-12-05'),
    ('Martin Kleppmann', 'Germany', '1983-03-10'),
    ('Eric Evans', 'US', '1960-01-01'),
    ('Kathy Sierra', 'US', '1957-01-01'),
    ('Joshua Bloch', 'US', '1961-08-28');

INSERT INTO books (title, isbn, pages, price, genre, published_at, metadata) VALUES
    ('Clean Code', '978-0132350884', 464, 33.99, 'Software Engineering', '2008-08-01',
     '{"edition": 1, "tags": ["clean code", "best practices"]}'),
    ('Designing Data-Intensive Applications', '978-1449373320', 616, 39.99, 'Distributed Systems', '2017-03-16',
     '{"edition": 1, "tags": ["distributed", "databases", "streams"]}'),
    ('Domain-Driven Design', '978-0321125217', 560, 52.99, 'Software Engineering', '2003-08-20',
     '{"edition": 1, "tags": ["DDD", "architecture"]}'),
    ('Head First Java', '978-0596009205', 720, 28.99, 'Programming', '2005-02-09',
     '{"edition": 2, "tags": ["java", "beginner"]}'),
    ('Effective Java', '978-0134685991', 416, 36.99, 'Programming', '2018-01-06',
     '{"edition": 3, "tags": ["java", "best practices"]}');
```

### Exercises

Try these yourself before looking at solutions:

```
1.  Select all books, showing only title, genre, and price.
2.  Find all books cheaper than ₹3000 (assume price is in USD, multiply by 83).
3.  Find books in the 'Programming' genre, ordered by price ascending.
4.  Find books published after 2010.
5.  Find the most expensive book.
6.  Count how many books are in each genre.
7.  Find the average page count of all books.
8.  Find genres where the average price is above 35.
9.  Update the price of 'Clean Code' to 29.99 and return the updated row.
10. Insert a new book. If the ISBN already exists, update the price instead.
11. Find all authors whose name contains 'Martin' (case-insensitive).
12. Find books where metadata contains 'databases' in the tags.
13. Select the title and how many days ago each book was published.
```

### Solutions

<details>
<summary>Click to reveal solutions</summary>

```sql
-- 1
SELECT title, genre, price FROM books;

-- 2
SELECT title, price, (price * 83)::NUMERIC(10,2) AS price_inr
FROM books WHERE price * 83 < 3000;

-- 3
SELECT * FROM books WHERE genre = 'Programming' ORDER BY price ASC;

-- 4
SELECT title, published_at FROM books WHERE published_at > '2010-01-01';

-- 5
SELECT title, price FROM books ORDER BY price DESC LIMIT 1;

-- 6
SELECT genre, COUNT(*) AS book_count FROM books GROUP BY genre;

-- 7
SELECT ROUND(AVG(pages), 0) AS avg_pages FROM books;

-- 8
SELECT genre, ROUND(AVG(price), 2) AS avg_price
FROM books GROUP BY genre HAVING AVG(price) > 35;

-- 9
UPDATE books SET price = 29.99, updated_at = NOW()
WHERE title = 'Clean Code'
RETURNING *;
-- (Note: we'd need to add updated_at column first, or remove it from the query)

-- 10
INSERT INTO books (title, isbn, pages, price, genre, published_at)
VALUES ('Clean Code', '978-0132350884', 464, 31.99, 'Software Engineering', '2008-08-01')
ON CONFLICT (isbn)
DO UPDATE SET price = EXCLUDED.price;

-- 11
SELECT * FROM authors WHERE name ILIKE '%martin%';

-- 12
SELECT title, metadata->'tags' AS tags
FROM books
WHERE metadata->'tags' @> '"databases"'::JSONB;

-- 13
SELECT title, (CURRENT_DATE - published_at) AS days_since_published
FROM books ORDER BY days_since_published;
```

</details>

---

## Quick Reference Cheat Sheet

```sql
-- Database
CREATE DATABASE mydb;
DROP DATABASE IF EXISTS mydb;
\c mydb

-- Table
CREATE TABLE t (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
ALTER TABLE t ADD COLUMN age INT;
DROP TABLE IF EXISTS t CASCADE;

-- CRUD
INSERT INTO t (name) VALUES ('x') RETURNING *;
SELECT * FROM t WHERE name = 'x' ORDER BY id LIMIT 10;
UPDATE t SET name = 'y' WHERE id = 1 RETURNING *;
DELETE FROM t WHERE id = 1 RETURNING *;

-- Upsert
INSERT INTO t (name) VALUES ('x')
ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name;

-- Aggregates
SELECT col, COUNT(*), AVG(num), SUM(num) FROM t
GROUP BY col HAVING COUNT(*) > 1 ORDER BY COUNT(*) DESC;

-- Casting
SELECT '42'::INT, NOW()::DATE, 5::NUMERIC / 2;
```

---

> **Next up → [Level 2: Relationships & Joins](./level-02-relationships-joins.md)** — where we connect tables together and unlock the real power of relational databases.
