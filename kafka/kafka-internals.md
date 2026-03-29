# Apache Kafka Internals — A Deep Dive (Spring Boot Edition)

> A comprehensive guide covering every important internal detail of Apache Kafka, from how messages are stored on disk to how consumers rebalance — all with practical Spring Boot context.

---

## Table of Contents

1. [What is Kafka & Why It Exists](#1-what-is-kafka--why-it-exists)
2. [High-Level Architecture](#2-high-level-architecture)
3. [Core Concepts](#3-core-concepts)
4. [Broker Internals](#4-broker-internals)
5. [Producer Internals](#5-producer-internals)
6. [Consumer Internals](#6-consumer-internals)
7. [Replication & Fault Tolerance](#7-replication--fault-tolerance)
8. [Storage Internals](#8-storage-internals)
9. [Exactly-Once Semantics (EOS)](#9-exactly-once-semantics-eos)
10. [Performance Secrets](#10-performance-secrets)
11. [ZooKeeper vs KRaft](#11-zookeeper-vs-kraft)
12. [Schema Registry](#12-schema-registry)
13. [Spring Boot + Kafka Integration](#13-spring-boot--kafka-integration)
14. [Common Pitfalls & Production Tips](#14-common-pitfalls--production-tips)

---

## 1. What is Kafka & Why It Exists

Apache Kafka is a **distributed event streaming platform** designed for:

| Trait               | What it means                                           |
| ------------------- | ------------------------------------------------------- |
| **High throughput** | Millions of messages/sec on commodity hardware          |
| **Low latency**     | Single-digit millisecond end-to-end                     |
| **Durability**      | Messages persisted to disk, replicated across brokers   |
| **Scalability**     | Partitions allow horizontal scaling of reads and writes |
| **Decoupling**      | Producers and consumers are fully independent           |

### Traditional Queue vs Kafka

```
Traditional Queue (RabbitMQ, ActiveMQ):
  Producer → Queue → Consumer  (message deleted after consumption)

Kafka:
  Producer → Topic (Log) → Consumer Group A  (message retained)
                          → Consumer Group B  (same message, independent)
```

**Key insight:** Kafka is a **distributed commit log**, NOT a traditional message queue. Messages are **appended** and **retained** — consumers simply track their position (offset) in the log.

---

## 2. High-Level Architecture

```
                        ┌─────────────────────────────────┐
                        │          Kafka Cluster           │
                        │                                  │
  ┌──────────┐          │  ┌─────────┐  ┌─────────┐       │         ┌──────────────┐
  │ Producer │──────────┼─▶│Broker 0 │  │Broker 1 │       │────────▶│ Consumer     │
  │ (Spring  │          │  │         │  │         │       │         │ Group A      │
  │  Boot)   │          │  │ P0-Lead │  │ P1-Lead │       │         │ (Spring Boot)│
  └──────────┘          │  │ P1-Foll │  │ P0-Foll │       │         └──────────────┘
                        │  └─────────┘  └─────────┘       │
  ┌──────────┐          │                                  │         ┌──────────────┐
  │ Producer │──────────┼─▶  ┌─────────┐                   │────────▶│ Consumer     │
  │ (Any)    │          │    │Broker 2 │                   │         │ Group B      │
  └──────────┘          │    │ P2-Lead │                   │         └──────────────┘
                        │    │ P0-Foll │                   │
                        │    └─────────┘                   │
                        │                                  │
                        │  ┌───────────────────────┐       │
                        │  │ ZooKeeper / KRaft      │       │
                        │  │ (cluster metadata)     │       │
                        │  └───────────────────────┘       │
                        └─────────────────────────────────┘
```

**Components at a glance:**

| Component             | Role                                                                                                            |
| --------------------- | --------------------------------------------------------------------------------------------------------------- |
| **Broker**            | A single Kafka server. Stores data, serves clients.                                                             |
| **Cluster**           | A group of brokers working together.                                                                            |
| **ZooKeeper / KRaft** | Manages cluster metadata, leader elections. KRaft (Kafka Raft) is the newer built-in replacement for ZooKeeper. |
| **Producer**          | Pushes records to topics.                                                                                       |
| **Consumer**          | Pulls records from topics.                                                                                      |
| **Consumer Group**    | A set of consumers that cooperatively consume from a topic.                                                     |

---

## 3. Core Concepts

### 3.1 Topics

A **topic** is a named category/feed of records. Think of it as a table name in a database.

```
Topic: "order-events"
Topic: "user-signups"
Topic: "payment-processed"
```

- Topics are **purely logical** — they don't map to a single file or disk location.
- A topic is split into **partitions**.

### 3.2 Partitions

A **partition** is an **ordered, immutable, append-only** sequence of records. It is the fundamental unit of parallelism.

```
Topic "orders" with 3 partitions:

Partition 0: [0] [1] [2] [3] [4] [5] ...  ← offset
Partition 1: [0] [1] [2] [3] ...
Partition 2: [0] [1] [2] ...
                 ↑
              each cell = one record
```

**Critical rules:**

- **Ordering is guaranteed ONLY within a partition**, not across partitions.
- Each partition lives on **one broker** (its leader), with replicas on others.
- A consumer in a group reads from **one or more partitions**, but a partition is read by **at most one consumer** in the same group.

### 3.3 Records (Messages)

A single unit of data in Kafka:

```
┌──────────────────────────────────────────────┐
│                  Record                       │
├──────────┬───────────┬───────────┬────────────┤
│  Key     │  Value    │ Timestamp │  Headers   │
│ (bytes)  │ (bytes)   │ (long)    │ (k/v list) │
└──────────┴───────────┴───────────┴────────────┘
```

| Field         | Purpose                                                                                                           |
| ------------- | ----------------------------------------------------------------------------------------------------------------- |
| **Key**       | Determines partition assignment (hash). `null` key → round-robin. Same key → same partition → ordering guarantee. |
| **Value**     | The actual payload (JSON, Avro, Protobuf, etc.)                                                                   |
| **Timestamp** | CreateTime (producer-set) or LogAppendTime (broker-set)                                                           |
| **Headers**   | Metadata key-value pairs (tracing IDs, content-type, etc.)                                                        |

### 3.4 Offsets

An **offset** is a **monotonically increasing integer** assigned to each record within a partition. It uniquely identifies a record within that partition.

```
Partition 0:  offset 0 → offset 1 → offset 2 → ... → offset N
```

- Offsets are **never reused** (even after deletion/compaction of older records).
- Consumers track their position by committing offsets.
- Three important offset positions per consumer:
  - **Last Committed Offset** — where the consumer will resume from on restart
  - **Current Position** — the next record to be fetched
  - **Log-End Offset (LEO)** — the offset of the newest record in the partition
  - **Lag** = LEO − Last Committed Offset

### 3.5 Consumer Groups

A **consumer group** is a set of consumers identified by a `group.id`. The partitions of a subscribed topic are **distributed** among the group members.

```
Topic "orders" (6 partitions) + Consumer Group "order-service" (3 consumers):

  Consumer A  ← reads P0, P1
  Consumer B  ← reads P2, P3
  Consumer C  ← reads P4, P5
```

**Rules:**

- Each partition → exactly one consumer in the group.
- If consumers > partitions → some consumers sit idle.
- If consumers < partitions → some consumers handle multiple partitions.
- Different consumer groups are **fully independent** — they each get all the messages.

---

## 4. Broker Internals

### 4.1 What a Broker Does

Each broker in the cluster:

1. **Accepts** produce and fetch requests from clients.
2. **Stores** records to local disk (in log segments).
3. **Replicates** data to/from other brokers (follower replicas fetch from leader).
4. **Serves** metadata to producers/consumers (partition leaders, ISR lists).

### 4.2 Controller Broker

One broker is elected as the **Controller**. It handles:

- **Partition leader election** (when a broker dies).
- **Reassigning partitions** (when brokers join/leave).
- **Topic creation/deletion** propagation.
- **ISR list management.**

In ZooKeeper mode, the controller is elected via a ZK ephemeral node. In KRaft mode, it's elected via the Raft consensus protocol.

---

## 5. Producer Internals

### 5.1 Lifecycle of a Produced Message

```
  producer.send(record)
        │
        ▼
  ┌──────────────┐
  │  Interceptors │  (ProducerInterceptor chain)
  └──────┬───────┘
         ▼
  ┌──────────────┐
  │  Serializer  │  Key + Value serialized to byte[]
  └──────┬───────┘
         ▼
  ┌──────────────┐
  │ Partitioner  │  Decides which partition the record goes to
  └──────┬───────┘
         ▼
  ┌──────────────────────────────────────┐
  │         Record Accumulator           │
  │  ┌────────────┐  ┌────────────┐      │
  │  │ Batch (P0) │  │ Batch (P1) │ ...  │  Records grouped by partition
  │  └────────────┘  └────────────┘      │
  └──────────────┬───────────────────────┘
                 │  (batch.size reached OR linger.ms elapsed)
                 ▼
  ┌──────────────────┐
  │   Sender Thread  │  Picks batches, groups by broker, sends
  └──────┬───────────┘
         │
         ▼
     Network I/O → Broker
```

### 5.2 Partitioning Strategy

| Scenario           | Behavior                                                                                                    |
| ------------------ | ----------------------------------------------------------------------------------------------------------- |
| Key is `null`      | **Sticky partitioner** (Kafka 2.4+). Optimizes batching by sticking to a partition until the batch is full. |
| Key is non-null    | `hash(key) % numPartitions` — guarantees same key → same partition (if partition count is constant)         |
| Custom partitioner | Your `Partitioner.partition()` implementation                                                               |

#### The Sticky Partitioner (KIP-480)

Introduced as the default in Kafka 2.4+ for records with **null keys**, the Sticky Partitioner is designed to improve batching efficiency:

- Instead of the older round-robin approach (which led to many small, inefficient batches), it **"sticks"** to a single partition until that partition's batch is full (`batch.size`) or `linger.ms` expires.
- Once the batch is sent, it selects a new random partition to "stick" to.

**Why it matters:** It dramatically reduces latency and lowers broker/producer CPU usage by sending fewer but larger requests, while still distributing records evenly across all partitions over time.

**Spring Boot configuration:**

```yaml
spring:
  kafka:
    producer:
      key-serializer: org.apache.kafka.common.serialization.StringSerializer
      value-serializer: org.apache.kafka.common.serialization.StringSerializer
      properties:
        partitioner.class: org.apache.kafka.clients.producer.RoundRobinPartitioner
```

### 5.3 Batching — `batch.size` & `linger.ms`

Kafka producers **don't send messages one by one**. They accumulate them in a **RecordBatch** per partition.

| Config          | Default          | What it does                                                                                  |
| --------------- | ---------------- | --------------------------------------------------------------------------------------------- |
| `batch.size`    | 16384 (16 KB)    | Max bytes per batch. Batch sent when this is full.                                            |
| `linger.ms`     | 0                | How long to wait for more records before sending an incomplete batch. `0` = send immediately. |
| `buffer.memory` | 33554432 (32 MB) | Total buffer memory for all unsent batches. If exhausted, `send()` blocks for `max.block.ms`. |

**How they interact:**

```
Record arrives → placed in batch for its partition
  ├── Batch is full (>= batch.size)?  → Send immediately
  ├── linger.ms timer expired?        → Send whatever is in the batch
  └── Otherwise                       → Wait for more records
```

> **Note:** Records that arrive close together in time will generally batch together even with `linger.ms=0`, so under heavy load batching will occur regardless of the linger configuration.

> **Tip:** In Spring Boot, set `linger.ms=5` to `20` and increase `batch.size` for higher throughput at the cost of slight latency.

```yaml
spring:
  kafka:
    producer:
      properties:
        linger.ms: 10
        batch.size: 32768 # 32 KB
```

### 5.4 Acknowledgements — `acks`

This determines **when the broker considers a write successful**:

| Value                | Behavior                                               | Durability                            | Speed   |
| -------------------- | ------------------------------------------------------ | ------------------------------------- | ------- |
| `acks=0`             | Don't wait for any acknowledgment                      | **Lowest** (fire-and-forget)          | Fastest |
| `acks=1`             | Wait for leader to write to its local log              | **Medium** (leader crash → data loss) | Fast    |
| `acks=all` (or `-1`) | Wait for **all in-sync replicas (ISR)** to acknowledge | **Highest**                           | Slowest |

#### How `acks` Affects Latency

- **Producer Latency:** The time the _producer_ waits for a response from the broker. `acks=all` increases producer latency because the leader must wait for all in-sync replicas (ISR) to acknowledge the write before responding to the producer. `acks=1` has lower producer latency because the leader responds immediately after writing to its own log.
- **End-to-End Latency:** The time from when a message is produced to when it is available for a _consumer_ to read. **Crucially, consumers can only read messages up to the High Watermark (messages replicated to all ISRs).** Therefore, `acks=1` and `acks=all` have virtually the **same end-to-end latency**. Even if the producer receives a faster acknowledgment with `acks=1`, the consumer still must wait for the replication to finish before the message becomes visible.

> **Always use `acks=all` for production systems.** Combine with `min.insync.replicas=2` on the topic/broker. It maximizes durability without penalizing consumer end-to-end latency!

```yaml
spring:
  kafka:
    producer:
      acks: all
```

### 5.5 Retries, Idempotence, & Ordering

| Config                                  | Default                           | Purpose                                                       |
| --------------------------------------- | --------------------------------- | ------------------------------------------------------------- |
| `retries`                               | 2147483647 (effectively infinite) | Number of times to retry a failed send                        |
| `retry.backoff.ms`                      | 100                               | Delay between retries                                         |
| `delivery.timeout.ms`                   | 120000 (2 min)                    | Total time from `send()` to success/failure including retries |
| `max.in.flight.requests.per.connection` | 5                                 | How many unacknowledged requests per broker connection        |
| `enable.idempotence`                    | `true` (Kafka 3.0+)               | Prevents duplicates and preserves ordering during retries     |

#### The Idempotent Producer

Without idempotence, a network timeout during an acknowledgment can cause the producer to retry a successfully written batch, creating **duplicate messages**.

When `enable.idempotence=true` (default since Kafka 3.0):

- **Deduplication:** The broker assigns the producer a **Producer ID (PID)** and tracks the **sequence number** of each message. If a retry sends an already-written sequence number, the broker safely ignores the duplicate.
- **Enforced Configs:** Enabling it automatically requires `acks=all`, `retries > 0`, and `max.in.flight.requests.per.connection <= 5` to ensure data safety and strict ordering.

#### Message Ordering Guarantees

If `max.in.flight.requests.per.connection` > 1 without idempotence, a failed batch that is being retried might be written _after_ a later batch that succeeded on the first try, **breaking message order**.

**With Idempotence enabled:**
The broker uses the sequence numbers to detect out-of-order batches. If Batch 2 (seq=2) arrives before a retried Batch 1 (seq=1), the broker buffers Batch 2 and waits for Batch 1 to succeed first, **guaranteeing order**. (This guarentee holds as long as `max.in.flight.requests.per.connection` is less than or equal to 5).

> **Important:** Kafka ordering is **strictly per-partition**. To ensure related messages are processed in order across your system, always publish them with the **same message key** so they land in the same partition. _(Note: This key-to-partition mapping is only consistent as long as the total number of partitions does not change!)_

```yaml
spring:
  kafka:
    producer:
      properties:
        enable.idempotence: true # Default since Kafka 3.0
        max.in.flight.requests.per.connection: 5
```

### 5.6 Compression

Messages can be compressed at the producer level (and optionally recompressed at the broker):

| Type     | Ratio | CPU     | Notes                                     |
| -------- | ----- | ------- | ----------------------------------------- |
| `none`   | —     | —       | No compression                            |
| `gzip`   | Best  | Highest | Good for text-heavy payloads              |
| `snappy` | Good  | Low     | Best balance for most use cases           |
| `lz4`    | Good  | Lowest  | Fastest compression                       |
| `zstd`   | Best  | Medium  | Best ratio with decent speed (Kafka 2.1+) |

```yaml
spring:
  kafka:
    producer:
      compression-type: snappy
```

> **Internal detail:** Compression happens at the **batch level**, not per-message. This is one reason batching helps — more records per batch = better compression ratio.

---

## 6. Consumer Internals

### 6.1 Poll Loop — The Heart of a Consumer

Kafka consumers use a **pull model**. The consumer calls `poll()` in a loop:

```java
while (true) {
    ConsumerRecords<K, V> records = consumer.poll(Duration.ofMillis(100));
    for (ConsumerRecord<K, V> record : records) {
        process(record);
    }
}
```

In Spring Boot with `@KafkaListener`, this loop is **managed for you** by the `KafkaMessageListenerContainer`.

```java
@KafkaListener(topics = "orders", groupId = "order-service")
public void listen(ConsumerRecord<String, String> record) {
    // This is called for each record inside the managed poll loop
    System.out.println("Key: " + record.key() + " Value: " + record.value());
}
```

### 6.2 Consumer Group Coordinator & Rebalancing

#### Group Coordinator

Each consumer group has a **Group Coordinator** — a specific broker responsible for managing the group's membership and partition assignments.

```
How the coordinator is found:
  hash(group.id) % __consumer_offsets partitions → partition number
  Leader of that partition → Group Coordinator
```

#### Why rebalancing is needed?

When a consumer joins or leaves the group, the partitions need to be redistributed among the remaining consumers.
If a consumer fails to send a heartbeat to the coordinator within the `session.timeout.ms` period, it is considered dead and a rebalance is triggered.
If a consumer closes cleanly, it will send a leave group request to the coordinator and a rebalance will be triggered immediately.

#### Rebalancing Protocol (Eager — Old Default)

```
All consumers stop consuming, give up their ownership of all partitions, rejoin the consumer group, and get a brand-new partition assignment. This is essentially a short window of unavailability of the entire consumer group. The length of the window depends on the size of the consumer group as well as on several configuration parameters.
```

#### Cooperative Rebalancing (New Default — Kafka 2.4+)

```
Instead of revoking ALL partitions and reassigning:
1. Only partitions that need to move are revoked
2. Other consumers continue consuming unaffected partitions
3. Revoked partitions are assigned in a second rebalance round
4. This process might take two or more rounds to achieve the stable assignment.

Result: Minimal disruption, near-zero downtime
```

**Spring Boot configuration:**

```yaml
spring:
  kafka:
    consumer:
      properties:
        partition.assignment.strategy: org.apache.kafka.clients.consumer.CooperativeStickyAssignor
```

### 6.3 Static Group Membership

Every consumer in a group has a `group.instance.id` configuration property:

- **If not set:** The broker assigns a random, unique member ID on startup. If the consumer restarts, it gets a new ID, triggering an immediate rebalance.
- **If set:** The consumer becomes a **static member**.

When a static member leaves or disconnects, the broker will wait for the `session.timeout.ms` duration before triggering a rebalance. If the consumer reconnects within that timeframe, it gets the **same partitions back without a rebalance**.

```yaml
spring:
  kafka:
    consumer:
      properties:
        group.instance.id: order-consumer-1 # should be unique per instance
        session.timeout.ms: 60000 # wait 60s before rebalancing if consumer dies
```

### 6.4 Offset Management

#### Where Are Offsets Stored?

Offsets are stored in a special internal Kafka topic: **`__consumer_offsets`** (50 partitions by default).

```
Key:   (group.id, topic, partition)
Value: (offset, metadata, timestamp)
```

#### Commit Strategies

| Strategy                | How                                                                | Trade-off                                          |
| ----------------------- | ------------------------------------------------------------------ | -------------------------------------------------- |
| **Auto-commit**         | Offsets committed every `auto.commit.interval.ms` (default 5000ms) | Simple but can lose or reprocess messages on crash |
| **Manual sync commit**  | `consumer.commitSync()`                                            | Blocks until committed. Safest.                    |
| **Manual async commit** | `consumer.commitAsync()`                                           | Non-blocking. May lose commits on failure.         |

**Spring Boot — AckMode options:**

```yaml
spring:
  kafka:
    consumer:
      enable-auto-commit: false # Recommended: let Spring manage it
    listener:
      ack-mode: MANUAL_IMMEDIATE # or RECORD, BATCH, TIME, COUNT, MANUAL
```

| AckMode            | Behavior                                                                                     |
| ------------------ | -------------------------------------------------------------------------------------------- |
| `RECORD`           | Commit after each record is processed                                                        |
| `BATCH`            | Commit after all records from `poll()` are processed (default)                               |
| `TIME`             | Commit after a time interval                                                                 |
| `COUNT`            | Commit after N records                                                                       |
| `MANUAL`           | You call `acknowledgment.acknowledge()` yourself, committed when next poll or container stop |
| `MANUAL_IMMEDIATE` | Commits immediately when you call `acknowledge()`                                            |

### 6.5 Fetch Internals

When a consumer calls `poll()`, here's what actually happens:

```
poll(timeout)
  │
  ├── Are there records in the local fetch buffer?
  │     YES → return them immediately
  │     NO  → send Fetch request to broker(s), wait up to timeout
  │
  └── Meanwhile, a background Fetcher thread pre-fetches from brokers
```

| Config                      | Default        | Purpose                                                                   |
| --------------------------- | -------------- | ------------------------------------------------------------------------- |
| `fetch.min.bytes`           | 1              | Min data the broker should return. Higher = fewer requests, more latency. |
| `fetch.max.wait.ms`         | 500            | Max time broker waits to accumulate `fetch.min.bytes`.                    |
| `max.partition.fetch.bytes` | 1048576 (1 MB) | Max data per partition per fetch.                                         |
| `max.poll.records`          | 500            | Max records returned per `poll()` call.                                   |
| `max.poll.interval.ms`      | 300000 (5 min) | Max time between `poll()` calls before consumer is considered dead.       |

> **Critical:** If your processing takes longer than `max.poll.interval.ms`, the coordinator will **kick the consumer out** of the group, triggering a rebalance. Tune this carefully.

---

## 7. Replication & Fault Tolerance

### 7.1 Replication Factor

Each partition has multiple **replicas** across brokers.

```
Topic "orders", replication-factor=3, partitions=2:

  Partition 0:  Broker 0 (Leader)  |  Broker 1 (Follower)  |  Broker 2 (Follower)
  Partition 1:  Broker 1 (Leader)  |  Broker 2 (Follower)  |  Broker 0 (Follower)
```

- Only the **Leader** handles produce and consume requests.
- **Followers** replicate data from the leader by sending Fetch requests (same protocol as consumers).

### 7.2 In-Sync Replicas (ISR)

A replica is **in-sync** if:

1. It has an active session with the controller (heartbeat within `zookeeper.session.timeout.ms` or equivalent).
2. It has fetched data from the leader within **`replica.lag.time.max.ms`** (default 30 seconds).

```
ISR = { Leader, Follower-1, Follower-2 }   # all caught up

If Follower-2 falls behind:
ISR = { Leader, Follower-1 }               # Follower-2 removed from ISR
```

When `acks=all`, the producer waits for **all replicas in the ISR** to acknowledge.

### 7.3 `min.insync.replicas`

A **topic or broker-level** setting that specifies the minimum number of replicas that must be in the ISR for a produce request with `acks=all` to succeed.

```
replication.factor = 3
min.insync.replicas = 2
acks = all

Scenario: 1 broker down → ISR has 2 replicas → writes succeed ✓
Scenario: 2 brokers down → ISR has 1 replica → writes REJECTED with NotEnoughReplicasException ✗
```

> **Golden rule:** `replication.factor = 3, min.insync.replicas = 2, acks = all` — this survives the loss of any single broker with zero data loss.

### 7.4 Leader Election

When a partition leader dies:

1. The **Controller** detects the broker's absence (via ZooKeeper session or KRaft heartbeat).
2. Controller selects a new leader from the **ISR list** (first replica in ISR by default).
3. Controller updates the metadata on all brokers.
4. Producers and consumers automatically discover the new leader via metadata refresh.

**Unclean leader election** (`unclean.leader.election.enable=false` by default):

- If ALL ISR replicas are dead, the partition goes **offline** rather than promoting an out-of-sync replica.
- Setting this to `true` risks data loss but maintains availability.

### 7.5 High Watermark (HW)

```
Leader log:     [0] [1] [2] [3] [4] [5]
                                     ↑ LEO (Log End Offset)
                               ↑ HW (High Watermark)

Follower 1:     [0] [1] [2] [3] [4]
Follower 2:     [0] [1] [2] [3]
```

- **HW** = the offset up to which **all ISR replicas** have confirmed the data.
- **Consumers can only read up to the HW**, not the LEO.
- This ensures a consumer never reads data that could be lost if the leader crashes.

---

## 8. Storage Internals

### 8.1 Log Directory Structure

Each partition is a directory on disk:

```
/kafka-logs/
  └── orders-0/                    # Topic "orders", Partition 0
        ├── 00000000000000000000.log     # Segment file (actual data)
        ├── 00000000000000000000.index    # Offset → physical position index
        ├── 00000000000000000000.timeindex # Timestamp → offset index
        ├── 00000000000054321000.log     # Next segment (starts at offset 54321000)
        ├── 00000000000054321000.index
        ├── 00000000000054321000.timeindex
        ├── leader-epoch-checkpoint
        └── partition.metadata
```

### 8.2 Log Segments

A partition's log is split into **segments**:

- **Active segment**: The segment currently being written to (only one per partition).
- **Closed segments**: Immutable, eligible for compaction/deletion.
- New segment created when active segment reaches `log.segment.bytes` (default 1 GB) or `log.roll.ms`/`log.roll.hours`.

> **File naming:** The filename is the **base offset** of the first record in that segment, zero-padded to 20 digits.

### 8.3 Index Files

To find a specific offset without scanning the entire segment, Kafka uses **sparse indexes**:

**Offset Index** (`.index`):

```
Relative Offset → Physical Position (byte offset in .log file)
4                → 320
8                → 640
12               → 960
...
```

**Timestamp Index** (`.timeindex`):

```
Timestamp         → Relative Offset
1677654321000     → 4
1677654325000     → 8
...
```

Lookup process:

1. **Binary search** the segment files to find the right segment.
2. **Binary search** the `.index` to find the nearest offset ≤ target.
3. **Sequential scan** from that position in the `.log` file.

### 8.4 Record Batch Format (On-Disk)

```
RecordBatch:
  ┌─────────────────────────────────────────────────────────┐
  │ BaseOffset (8 bytes)                                     │
  │ BatchLength (4 bytes)                                    │
  │ PartitionLeaderEpoch (4 bytes)                           │
  │ Magic (1 byte) — format version (currently 2)            │
  │ CRC (4 bytes) — checksum of everything after this        │
  │ Attributes (2 bytes) — compression, timestamp type, etc  │
  │ LastOffsetDelta (4 bytes)                                │
  │ FirstTimestamp (8 bytes)                                 │
  │ MaxTimestamp (8 bytes)                                   │
  │ ProducerId (8 bytes) — for idempotence/transactions      │
  │ ProducerEpoch (2 bytes)                                  │
  │ BaseSequence (4 bytes)                                   │
  │ Records[]                                                │
  └─────────────────────────────────────────────────────────┘
```

### 8.5 Retention Policies

| Policy         | Config                                     | Default            | Behavior                                                |
| -------------- | ------------------------------------------ | ------------------ | ------------------------------------------------------- |
| **Time-based** | `log.retention.hours` / `log.retention.ms` | 168 hours (7 days) | Delete segments older than this                         |
| **Size-based** | `log.retention.bytes`                      | -1 (unlimited)     | Delete oldest segments when partition exceeds this size |
| **Compaction** | `log.cleanup.policy=compact`               | —                  | Keep only the **latest value per key**                  |

### 8.6 Log Compaction — Deep Dive

For topics with `cleanup.policy=compact`:

```
Before compaction:

  offset 0: key=A, val=1
  offset 1: key=B, val=2
  offset 2: key=A, val=3    ← newer value for key A
  offset 3: key=C, val=4
  offset 4: key=B, val=5    ← newer value for key B
  offset 5: key=A, val=null ← tombstone (delete marker for key A)

After compaction:

  offset 2: key=A, val=3    ← kept (but tombstone may remove this later)
  offset 3: key=C, val=4
  offset 4: key=B, val=5
  offset 5: key=A, val=null ← tombstone retained for delete.retention.ms
```

- **Only closed segments** are compacted. The active segment is never touched.
- **Tombstones** (key with `null` value) signal deletion and are retained for `delete.retention.ms` (default 24h).
- **Use case:** Changelog streams, KTable materializations, CDC.

---

## 9. Exactly-Once Semantics (EOS)

### 9.1 Delivery Guarantees Spectrum

| Guarantee         | Description                                    | How                                    |
| ----------------- | ---------------------------------------------- | -------------------------------------- |
| **At-most-once**  | Messages may be lost, never duplicated         | Consumer commits **before** processing |
| **At-least-once** | Messages are never lost, but may be duplicated | Consumer commits **after** processing  |
| **Exactly-once**  | Messages are neither lost nor duplicated       | Idempotent producer + Transactions     |

### 9.2 Transactional Producer

For Kafka-to-Kafka workflows (consume → process → produce), Kafka supports **transactions**:

```java
producer.initTransactions();
try {
    producer.beginTransaction();
    producer.send(new ProducerRecord<>("output-topic", key, value));
    producer.sendOffsetsToTransaction(offsets, consumerGroupMetadata);
    producer.commitTransaction();
} catch (Exception e) {
    producer.abortTransaction();
}
```

**What happens internally:**

1. Producer registers with a **Transaction Coordinator** (a broker).
2. Coordinator assigns a **transactional.id** → PID mapping (persisted in `__transaction_state` topic).
3. During the transaction, all produced records are "`uncommitted`."
4. On `commitTransaction()`, coordinator writes a **COMMIT marker** to all involved partitions.
5. Consumers with `isolation.level=read_committed` only see records with committed markers.

### 9.3 Spring Boot Transactional Producer

```yaml
spring:
  kafka:
    producer:
      transaction-id-prefix: tx-order- # enables transactions
```

```java
@Service
public class OrderService {

    private final KafkaTemplate<String, String> kafkaTemplate;

    public OrderService(KafkaTemplate<String, String> kafkaTemplate) {
        this.kafkaTemplate = kafkaTemplate;
    }

    public void processOrder(String orderId, String orderData) {
        kafkaTemplate.executeInTransaction(ops -> {
            ops.send("order-processed", orderId, orderData);
            ops.send("order-notification", orderId, "Order processed");
            return true;
        });
    }
}
```

---

## 10. Performance Secrets

### 10.1 Sequential I/O

Kafka writes to disk **sequentially** (append-only). Sequential disk writes are 6x faster than random writes, even faster than random memory access in some cases.

```
Random I/O:   ~100 MB/s (SSD), ~1 MB/s (HDD)
Sequential I/O: ~600 MB/s (SSD), ~100 MB/s (HDD)
```

### 10.2 Zero-Copy Transfer (sendfile)

When a consumer fetches data, Kafka uses the OS `sendfile()` system call:

```
Traditional path (4 copies):
  Disk → Kernel Buffer → User Buffer → Socket Buffer → NIC

Zero-copy path (2 copies):
  Disk → Kernel Buffer ─────────────────→ NIC
           (via DMA)      (via sendfile)
```

This eliminates 2 copies and 2 context switches, dramatically improving throughput.

### 10.3 Page Cache

Kafka **does not manage its own cache**. It relies entirely on the OS **page cache**:

- Writes go to page cache first (fsync is periodic, not per-message).
- Reads of recent data are served directly from page cache → no disk I/O.
- This is why Kafka can serve hot data at **memory speed** while persisting to disk.

> **Production tip:** Give Kafka brokers plenty of free RAM (not heap). The OS will use it for page cache. Example: 6 GB JVM heap + 26 GB free for page cache on a 32 GB machine.

### 10.4 Batching & Compression

As discussed in Section 5, batching allows:

- **Fewer network round-trips** (many records in one request).
- **Better compression** (compressing a batch vs individual records).
- **Fewer disk writes** (entire batch written as one unit).

### 10.5 Partitioning = Parallelism

- Each partition can be handled by **a separate consumer thread**.
- Write throughput scales linearly with partition count (up to broker limits).
- Read throughput scales by adding consumers (up to partition count per group).

---

## 11. ZooKeeper vs KRaft

### 11.1 ZooKeeper Mode (Legacy)

ZooKeeper stored:

- **Broker registration** (which brokers are alive).
- **Topic configuration** (partition count, replication factor).
- **Controller election** (which broker is the controller).
- **ACLs** (access control lists).

**Problems:**

- External dependency, separate deployment.
- Scalability bottleneck (ZK becomes a bottleneck at ~200K partitions).
- Operational complexity (managing 2 systems instead of 1).

### 11.2 KRaft Mode (Kafka 3.3+ production-ready, ZK removed in 4.0)

**KRaft** = Kafka Raft metadata mode. The metadata quorum is managed by Kafka brokers themselves.

```
KRaft Architecture:

  ┌────────────┐  ┌────────────┐  ┌────────────┐
  │ Controller │  │ Controller │  │ Controller │   (Raft quorum)
  │  (voter)   │  │  (voter)   │  │  (voter)   │
  └──────┬─────┘  └──────┬─────┘  └──────┬─────┘
         │               │               │
    Metadata log replicated via Raft protocol
         │               │               │
  ┌──────┴─────┐  ┌──────┴─────┐  ┌──────┴─────┐
  │  Broker 1  │  │  Broker 2  │  │  Broker 3  │
  └────────────┘  └────────────┘  └────────────┘
```

**Benefits:**

- No external dependency.
- Supports millions of partitions.
- Faster controller failover (seconds vs minutes).
- Simpler operations.

---

## 12. Schema Registry

While not part of Kafka itself, Schema Registry is essential in production:

```
Producer                                      Consumer
   │                                             │
   ├── 1. Register schema ─────┐                 │
   │                            ▼                 │
   │                     ┌──────────────┐         │
   │                     │   Schema     │         │
   │                     │  Registry    │◀────────┤ 3. Fetch schema
   │                     └──────────────┘         │
   │                            │                 │
   ├── 2. Serialize with        │                 ├── 4. Deserialize with
   │      schema ID ──▶ Kafka ──┼─────────────────┤     schema ID
   │                            │                 │
```

**Supported formats:** Avro, Protobuf, JSON Schema.

**Compatibility modes:**
| Mode | Rule |
|---|---|
| `BACKWARD` | New schema can read data written with the previous schema |
| `FORWARD` | Previous schema can read data written with the new schema |
| `FULL` | Both backward and forward compatible |
| `NONE` | No compatibility checks |

---

## 13. Spring Boot + Kafka Integration

### 13.1 Dependencies

```xml
<!-- pom.xml -->
<dependency>
    <groupId>org.springframework.kafka</groupId>
    <artifactId>spring-kafka</artifactId>
</dependency>
```

This brings in:

- `spring-kafka` — Spring's Kafka abstraction layer
- `kafka-clients` — The official Apache Kafka Java client

### 13.2 Application Configuration (Full Reference)

```yaml
spring:
  kafka:
    bootstrap-servers: localhost:9092

    producer:
      key-serializer: org.apache.kafka.common.serialization.StringSerializer
      value-serializer: org.springframework.kafka.support.serializer.JsonSerializer
      acks: all
      retries: 3
      compression-type: snappy
      properties:
        linger.ms: 10
        batch.size: 32768
        enable.idempotence: true
        max.in.flight.requests.per.connection: 5

    consumer:
      group-id: my-service
      key-deserializer: org.apache.kafka.common.serialization.StringDeserializer
      value-deserializer: org.springframework.kafka.support.serializer.JsonDeserializer
      auto-offset-reset: earliest # or 'latest'
      enable-auto-commit: false
      properties:
        spring.json.trusted.packages: "com.example.dto"
        partition.assignment.strategy: org.apache.kafka.clients.consumer.CooperativeStickyAssignor
        max.poll.records: 100
        max.poll.interval.ms: 300000
        session.timeout.ms: 45000
        heartbeat.interval.ms: 15000

    listener:
      ack-mode: MANUAL_IMMEDIATE
      concurrency: 3 # number of consumer threads
      type: single # or 'batch' for batch listeners
```

### 13.3 Spring Kafka Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                     Spring Boot Application                     │
│                                                                 │
│  ┌────────────────────┐        ┌──────────────────────────┐    │
│  │   KafkaTemplate    │        │  KafkaListenerContainer   │    │
│  │                    │        │  Factory                   │    │
│  │  ▪ send()          │        │                            │    │
│  │  ▪ sendDefault()   │        │  Creates for each          │    │
│  │  ▪ executeInTxn()  │        │  @KafkaListener:           │    │
│  └────────┬───────────┘        │                            │    │
│           │                    │  ┌──────────────────────┐  │    │
│           │                    │  │  MessageListener     │  │    │
│           ▼                    │  │  Container           │  │    │
│  ┌────────────────────┐       │  │  (manages poll loop) │  │    │
│  │  ProducerFactory   │       │  │                      │  │    │
│  │  (DefaultKafka     │       │  │  → calls your        │  │    │
│  │   ProducerFactory) │       │  │    @KafkaListener    │  │    │
│  └────────┬───────────┘       │  │    method            │  │    │
│           │                    │  └──────────────────────┘  │    │
│           ▼                    └──────────────────────────────┘    │
│  ┌────────────────────┐        ┌──────────────────────────┐    │
│  │  KafkaProducer     │        │  KafkaConsumer           │    │
│  │  (from kafka-      │        │  (from kafka-clients)    │    │
│  │   clients)         │        │                          │    │
│  └────────────────────┘        └──────────────────────────┘    │
└────────────────────────────────────────────────────────────────┘
```

### 13.4 Producer — KafkaTemplate

```java
@Service
public class OrderProducer {

    private final KafkaTemplate<String, OrderEvent> kafkaTemplate;

    public OrderProducer(KafkaTemplate<String, OrderEvent> kafkaTemplate) {
        this.kafkaTemplate = kafkaTemplate;
    }

    // Basic send
    public void sendOrder(OrderEvent order) {
        kafkaTemplate.send("orders", order.getId(), order);
    }

    // With callback
    public void sendOrderWithCallback(OrderEvent order) {
        CompletableFuture<SendResult<String, OrderEvent>> future =
            kafkaTemplate.send("orders", order.getId(), order);

        future.whenComplete((result, ex) -> {
            if (ex == null) {
                RecordMetadata metadata = result.getRecordMetadata();
                System.out.printf("Sent to partition %d, offset %d%n",
                    metadata.partition(), metadata.offset());
            } else {
                System.err.println("Failed to send: " + ex.getMessage());
            }
        });
    }

    // Sending with headers
    public void sendWithHeaders(OrderEvent order) {
        ProducerRecord<String, OrderEvent> record =
            new ProducerRecord<>("orders", null, order.getId(), order);
        record.headers().add("correlation-id",
            UUID.randomUUID().toString().getBytes());
        record.headers().add("source", "order-service".getBytes());

        kafkaTemplate.send(record);
    }
}
```

### 13.5 Consumer — @KafkaListener

```java
@Component
public class OrderConsumer {

    // Basic listener
    @KafkaListener(topics = "orders", groupId = "order-service")
    public void listen(ConsumerRecord<String, OrderEvent> record) {
        System.out.println("Received: " + record.value());
    }

    // With manual acknowledgment
    @KafkaListener(topics = "orders", groupId = "order-service")
    public void listenManual(ConsumerRecord<String, OrderEvent> record,
                              Acknowledgment acknowledgment) {
        try {
            processOrder(record.value());
            acknowledgment.acknowledge();  // commit offset
        } catch (Exception e) {
            // Don't acknowledge — record will be redelivered
            throw e;
        }
    }

    // Batch listener
    @KafkaListener(topics = "orders", groupId = "order-service",
                   containerFactory = "batchFactory")
    public void listenBatch(List<ConsumerRecord<String, OrderEvent>> records,
                             Acknowledgment acknowledgment) {
        for (ConsumerRecord<String, OrderEvent> record : records) {
            processOrder(record.value());
        }
        acknowledgment.acknowledge();  // commit all at once
    }

    // Listening to specific partitions with initial offset
    @KafkaListener(topicPartitions = @TopicPartition(
        topic = "orders",
        partitionOffsets = {
            @PartitionOffset(partition = "0", initialOffset = "0"),
            @PartitionOffset(partition = "1", initialOffset = "0")
        }
    ))
    public void listenFromBeginning(ConsumerRecord<String, OrderEvent> record) {
        System.out.println("Replaying: " + record.value());
    }

    // Using @Payload and @Header annotations
    @KafkaListener(topics = "orders", groupId = "order-service")
    public void listenWithHeaders(
            @Payload OrderEvent order,
            @Header(KafkaHeaders.RECEIVED_PARTITION) int partition,
            @Header(KafkaHeaders.OFFSET) long offset,
            @Header(KafkaHeaders.RECEIVED_TIMESTAMP) long timestamp) {
        System.out.printf("Order: %s, Partition: %d, Offset: %d%n",
            order, partition, offset);
    }
}
```

### 13.6 Error Handling & Dead Letter Topics (DLT)

```java
@Configuration
public class KafkaConfig {

    @Bean
    public ConcurrentKafkaListenerContainerFactory<String, String>
            kafkaListenerContainerFactory(
                ConsumerFactory<String, String> consumerFactory) {

        ConcurrentKafkaListenerContainerFactory<String, String> factory =
            new ConcurrentKafkaListenerContainerFactory<>();
        factory.setConsumerFactory(consumerFactory);

        // Retry 3 times with exponential backoff, then send to DLT
        factory.setCommonErrorHandler(new DefaultErrorHandler(
            new DeadLetterPublishingRecoverer(kafkaTemplate()),
            new ExponentialBackOff(1000L, 2.0)  // 1s, 2s, 4s
        ));

        return factory;
    }
}
```

When a consumer fails to process a message after retries, the message is sent to `<original-topic>.DLT`.

```
orders (original topic)
  ├── Consumer fails 3 times
  └── Message sent to → orders.DLT  (Dead Letter Topic)
```

### 13.7 Custom Serializer/Deserializer

```java
// Using JSON with type headers
@Configuration
public class KafkaProducerConfig {

    @Bean
    public ProducerFactory<String, Object> producerFactory() {
        Map<String, Object> config = new HashMap<>();
        config.put(ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, "localhost:9092");
        config.put(ProducerConfig.KEY_SERIALIZER_CLASS_CONFIG,
            StringSerializer.class);
        config.put(ProducerConfig.VALUE_SERIALIZER_CLASS_CONFIG,
            JsonSerializer.class);

        // Add type info to headers so consumer knows what class to deserialize to
        config.put(JsonSerializer.ADD_TYPE_INFO_HEADERS, true);

        return new DefaultKafkaProducerFactory<>(config);
    }
}

@Configuration
public class KafkaConsumerConfig {

    @Bean
    public ConsumerFactory<String, Object> consumerFactory() {
        Map<String, Object> config = new HashMap<>();
        config.put(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, "localhost:9092");
        config.put(ConsumerConfig.KEY_DESERIALIZER_CLASS_CONFIG,
            StringDeserializer.class);
        config.put(ConsumerConfig.VALUE_DESERIALIZER_CLASS_CONFIG,
            JsonDeserializer.class);

        // Trust packages for deserialization
        config.put(JsonDeserializer.TRUSTED_PACKAGES, "com.example.dto");

        // Use type headers from producer
        config.put(JsonDeserializer.USE_TYPE_INFO_HEADERS, true);

        return new DefaultKafkaConsumerFactory<>(config);
    }
}
```

### 13.8 Spring Kafka Concurrency Model

The `concurrency` setting creates multiple `KafkaMessageListenerContainer` instances, each running its own consumer thread:

```
concurrency=3 with topic having 6 partitions:

  Container Thread 0 → KafkaConsumer → polls P0, P1
  Container Thread 1 → KafkaConsumer → polls P2, P3
  Container Thread 2 → KafkaConsumer → polls P4, P5
```

> **rule:** Set `concurrency` ≤ partition count. Exceeding it wastes threads.

### 13.9 Testing with Embedded Kafka

```java
@SpringBootTest
@EmbeddedKafka(partitions = 1, topics = {"test-topic"})
class OrderProducerTest {

    @Autowired
    private KafkaTemplate<String, String> kafkaTemplate;

    @Autowired
    private EmbeddedKafkaBroker embeddedKafka;

    @Test
    void testSendMessage() throws Exception {
        kafkaTemplate.send("test-topic", "key", "value").get();

        Map<String, Object> consumerProps =
            KafkaTestUtils.consumerProps("test-group", "true", embeddedKafka);
        ConsumerFactory<String, String> cf =
            new DefaultKafkaConsumerFactory<>(consumerProps);
        Consumer<String, String> consumer = cf.createConsumer();
        embeddedKafka.consumeFromAnEmbeddedTopic(consumer, "test-topic");

        ConsumerRecord<String, String> record =
            KafkaTestUtils.getSingleRecord(consumer, "test-topic");

        assertEquals("key", record.key());
        assertEquals("value", record.value());
    }
}
```

---

## 14. Common Pitfalls & Production Tips

### 14.1 Partition Count — Choose Wisely

| Factor                   | Guidance                                                                                               |
| ------------------------ | ------------------------------------------------------------------------------------------------------ |
| **Throughput target**    | Measure single-partition throughput. `total_throughput / single_partition_throughput = min partitions` |
| **Consumer parallelism** | Max consumers in a group = partition count                                                             |
| **Ordering**             | More partitions = less ordering guarantees (ordering is per-partition only)                            |
| **Overhead**             | Each partition = open file handles, memory. Don't go above 10K per broker.                             |
| **Cannot reduce**        | You can add partitions but NEVER reduce them without recreating the topic.                             |

> **Rule of thumb:** Start with `6 × broker_count` partitions for a general-purpose topic.

### 14.2 Consumer Lag Monitoring

Consumer lag = difference between the latest offset and the consumer's committed offset.

```bash
# Check consumer lag with kafka CLI
kafka-consumer-groups.sh --bootstrap-server localhost:9092 \
  --describe --group my-service
```

In Spring Boot, expose lag via Micrometer:

```yaml
management:
  endpoints:
    web:
      exposure:
        include: health,metrics,prometheus
```

The metric `kafka.consumer.fetch.manager.records.lag.max` gives you the maximum lag.

### 14.3 message.max.bytes vs fetch.max.bytes

```
Producer side:
  max.request.size (default 1 MB) — max size of a produce request

Broker side:
  message.max.bytes (default 1 MB) — max record batch size the broker accepts

Consumer side:
  fetch.max.bytes (default 50 MB) — max size of a fetch response
  max.partition.fetch.bytes (default 1 MB) — max data per partition per fetch

ALL THREE MUST BE CONSISTENT. If producer sends a 5 MB message:
  ✓ max.request.size >= 5 MB
  ✓ message.max.bytes >= 5 MB
  ✓ max.partition.fetch.bytes >= 5 MB
```

### 14.4 Rebalance Storms

**Symptoms:** Consumers constantly joining/leaving the group, high latency.

**Causes & Fixes:**

| Cause                     | Fix                                                          |
| ------------------------- | ------------------------------------------------------------ |
| Processing takes too long | Increase `max.poll.interval.ms`, decrease `max.poll.records` |
| JVM GC pauses             | Tune GC, increase `session.timeout.ms`                       |
| Frequent deploys          | Use static group membership (`group.instance.id`)            |
| Eager rebalancing         | Switch to `CooperativeStickyAssignor`                        |

### 14.5 Ordering Guarantees

```
Need strict ordering for all messages?
  → Use 1 partition (limits throughput to ~single consumer speed)

Need ordering per entity (e.g., per user, per order)?
  → Use the entity ID as the message key
  → All messages for that entity go to the same partition
  → max.in.flight.requests.per.connection=1 for strict ordering
     (or use idempotent producer which handles this with up to 5)
```

### 14.6 Quick Reference — Common Exceptions

| Exception                      | Meaning                                      | Fix                                            |
| ------------------------------ | -------------------------------------------- | ---------------------------------------------- |
| `NotEnoughReplicasException`   | ISR count < `min.insync.replicas`            | Add brokers, wait for replicas to catch up     |
| `RecordTooLargeException`      | Message exceeds `message.max.bytes`          | Increase limits or compress/split messages     |
| `TimeoutException`             | `delivery.timeout.ms` expired                | Check broker health, network, increase timeout |
| `SerializationException`       | Cannot serialize/deserialize                 | Fix schema, check serializer config            |
| `CommitFailedException`        | Offset commit failed (rebalance in progress) | Handle in error handler, use manual commits    |
| `RebalanceInProgressException` | Consumer group is rebalancing                | Wait, use cooperative rebalancing              |

---

## Cheat Sheet — Config Quick Reference

### Producer

| Config                                  | Recommended | Why                          |
| --------------------------------------- | ----------- | ---------------------------- |
| `acks`                                  | `all`       | Maximum durability           |
| `enable.idempotence`                    | `true`      | Prevent duplicates           |
| `linger.ms`                             | `10-20`     | Better batching              |
| `batch.size`                            | `32768`     | 32 KB batches                |
| `compression.type`                      | `snappy`    | Good balance                 |
| `max.in.flight.requests.per.connection` | `5`         | Max allowed with idempotence |

### Consumer

| Config                          | Recommended                 | Why                              |
| ------------------------------- | --------------------------- | -------------------------------- |
| `enable.auto.commit`            | `false`                     | Let Spring manage commits        |
| `auto.offset.reset`             | `earliest`                  | Don't miss messages on first run |
| `max.poll.records`              | `100-500`                   | Control batch size per poll      |
| `max.poll.interval.ms`          | Based on processing         | Prevent unnecessary rebalances   |
| `partition.assignment.strategy` | `CooperativeStickyAssignor` | Smoother rebalances              |

### Broker / Topic

| Config                           | Recommended       | Why                                     |
| -------------------------------- | ----------------- | --------------------------------------- |
| `replication.factor`             | `3`               | Survive 1 broker failure                |
| `min.insync.replicas`            | `2`               | With acks=all, survive 1 broker failure |
| `unclean.leader.election.enable` | `false`           | Prevent data loss                       |
| `log.retention.hours`            | Based on use case | 7 days default, adjust as needed        |

---

> **Summary:** Kafka is a distributed commit log where producers append records to partitioned topics, which are replicated across brokers for fault tolerance. Consumers pull data in groups with partition-level parallelism. Its performance comes from sequential I/O, zero-copy transfers, page cache reliance, and smart batching. Spring Boot wraps all this in `KafkaTemplate` and `@KafkaListener`, handling the poll loop, offset management, serialization, and error recovery for you.
