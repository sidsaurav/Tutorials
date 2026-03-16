# Kafka DLT (Dead Letter Topic) Error Handling — Complete Deep Dive (Spring Boot Edition)

> A comprehensive guide covering every aspect of error handling in Kafka consumers with Spring Boot: from the moment a message fails processing, through retry strategies, to Dead Letter Topic routing, reprocessing, and monitoring.

---

## Table of Contents

1. [The Problem — Why Do We Need DLTs?](#1-the-problem--why-do-we-need-dlts)
2. [Spring Kafka Error Handling Architecture](#2-spring-kafka-error-handling-architecture)
3. [DefaultErrorHandler — Blocking Retries + DLT](#3-defaulterrorhandler--blocking-retries--dlt)
4. [DeadLetterPublishingRecoverer — Deep Dive](#4-deadletterpublishingrecoverer--deep-dive)
5. [Non-Blocking Retries — @RetryableTopic](#5-non-blocking-retries--retryabletopic)
6. [Exception Classification — Retryable vs Non-Retryable](#6-exception-classification--retryable-vs-non-retryable)
7. [DLT Headers — Metadata on Failed Records](#7-dlt-headers--metadata-on-failed-records)
8. [DLT Consumer Patterns — What to Do with Dead Letters](#8-dlt-consumer-patterns--what-to-do-with-dead-letters)
9. [Deserialization Errors — A Special Case](#9-deserialization-errors--a-special-case)
10. [Complete Production-Ready Example](#10-complete-production-ready-example)
11. [Non-Blocking Retry with @RetryableTopic — Full Example](#11-non-blocking-retry-with-retryabletopic--full-example)
12. [Blocking vs Non-Blocking — Decision Guide](#12-blocking-vs-non-blocking--decision-guide)
13. [DLT Monitoring & Metrics](#13-dlt-monitoring--metrics)
14. [Quick Reference — Error Handling Cheat Sheet](#14-quick-reference--error-handling-cheat-sheet)
15. [Common Mistakes](#15-common-mistakes)

---

## 1. The Problem — Why Do We Need DLTs?

When a Kafka consumer fails to process a message, you have a fundamental problem:

```
Consumer reads offset 42 → processing throws exception → what now?

Option A: Skip it (data loss — at-most-once)
Option B: Retry forever (blocks the entire partition — consumer lag grows)
Option C: Retry N times, then move the poison pill somewhere safe ← DLT
```

A **Dead Letter Topic (DLT)** is a separate Kafka topic where "poisonous" messages — those that repeatedly fail processing — are sent after exhausting all retry attempts. This lets the consumer **move on** while preserving the failed message for later analysis, manual fixing, or automated reprocessing.

```
┌──────────────┐    ┌──────────────┐    ┌──────────────────────┐
│  orders      │    │  Consumer    │    │  orders.DLT          │
│  (topic)     │───▶│  (retries    │───▶│  (dead letter topic) │
│              │    │   exhaust)   │    │                      │
└──────────────┘    └──────────────┘    └──────────────────────┘
                                               │
                                               ▼
                                        ┌──────────────────┐
                                        │ DLT Consumer     │
                                        │ (alert, log,     │
                                        │  reprocess)      │
                                        └──────────────────┘
```

**Key insight:** Without DLTs, a single bad message can block an entire partition indefinitely, causing consumer lag to grow unbounded.

---

## 2. Spring Kafka Error Handling Architecture

Spring Kafka's error handling has evolved significantly. Here's the modern architecture (Spring Kafka 2.8+):

```
@KafkaListener method throws exception
        │
        ▼
┌──────────────────────────────┐
│  CommonErrorHandler          │  ← interface (replaces deprecated
│  (DefaultErrorHandler)       │     SeekToCurrentErrorHandler &
│                              │     RetryingBatchErrorHandler)
│  ┌────────────────────────┐  │
│  │ BackOff Strategy       │  │  ← how long to wait between retries
│  │ (FixedBackOff,         │  │     (blocking retries within the
│  │  ExponentialBackOff)   │  │      consumer thread)
│  └────────────────────────┘  │
│                              │
│  ┌────────────────────────┐  │
│  │ ConsumerRecordRecoverer│  │  ← what to do when retries are
│  │ (DeadLetterPublishing  │  │     exhausted (send to DLT,
│  │  Recoverer)            │  │     log, custom action)
│  └────────────────────────┘  │
│                              │
│  ┌────────────────────────┐  │
│  │ Exception Classifier   │  │  ← which exceptions are retryable
│  │ (retryable vs fatal)   │  │     vs immediately fatal
│  └────────────────────────┘  │
└──────────────────────────────┘
```

### Evolution of Error Handlers in Spring Kafka

| Spring Kafka Version | Error Handler | Status |
|---|---|---|
| < 2.8 | `SeekToCurrentErrorHandler` | **Deprecated** |
| < 2.8 | `RetryingBatchErrorHandler` | **Deprecated** |
| 2.8+ | `DefaultErrorHandler` | **Current — use this** |
| 2.9+ | `@RetryableTopic` (non-blocking) | **Current — for advanced use** |

**Two retry approaches in Spring Kafka:**

| Approach | How | Pro | Con |
|---|---|---|---|
| **Blocking retry** (`DefaultErrorHandler`) | Retries within the same consumer thread using `BackOff` | Simple, ordering preserved | Blocks the partition during backoff; consumer lag grows |
| **Non-blocking retry** (`@RetryableTopic`) | Publishes to retry topics, consumed by separate listeners | Doesn't block partition; other records continue processing | Complex topic topology; ordering NOT preserved |

---

## 3. DefaultErrorHandler — Blocking Retries + DLT

This is the **most common** error handling setup. The consumer retries in-place, then sends to DLT.

### 3.1 How It Works Internally

```
Record at offset 42 fails
        │
        ▼
  Attempt 1 of N (with backoff delay between attempts)
        │
  Still failing?
        │
   YES ─┼── Attempt 2 ... Attempt N
        │
  All N attempts exhausted?
        │
   YES ─┼──▶ ConsumerRecordRecoverer.accept(record, exception)
        │         │
        │         ▼
        │    DeadLetterPublishingRecoverer
        │         │
        │         ├── Determines destination topic (default: <topic>.DLT)
        │         ├── Adds exception headers (stacktrace, cause, etc.)
        │         └── Publishes record to DLT via KafkaTemplate
        │
        ▼
  Consumer commits offset 42 and moves to offset 43
```

> **Critical detail:** During blocking retries, the consumer thread is **sleeping**. It is NOT polling Kafka. If the total retry time exceeds `max.poll.interval.ms`, the consumer will be **kicked out** of the group, triggering a rebalance.

### 3.2 Full Configuration Example

```java
@Configuration
public class KafkaErrorHandlingConfig {

    @Autowired
    private KafkaTemplate<String, Object> kafkaTemplate;

    @Bean
    public ConcurrentKafkaListenerContainerFactory<String, Object>
            kafkaListenerContainerFactory(
                ConsumerFactory<String, Object> consumerFactory) {

        ConcurrentKafkaListenerContainerFactory<String, Object> factory =
            new ConcurrentKafkaListenerContainerFactory<>();
        factory.setConsumerFactory(consumerFactory);

        // Configure the error handler
        DefaultErrorHandler errorHandler = new DefaultErrorHandler(
            // 1. Recoverer — what happens after retries are exhausted
            new DeadLetterPublishingRecoverer(kafkaTemplate),
            // 2. BackOff — retry timing (1s, 2s, 4s = 3 attempts)
            new ExponentialBackOff(1000L, 2.0)
        );

        // 3. Configure exception classification
        // These exceptions are NOT retried — sent to DLT immediately
        errorHandler.addNotRetryableExceptions(
            JsonParseException.class,
            DeserializationException.class,
            NullPointerException.class
        );

        // These exceptions ARE retried (default: all exceptions are retryable)
        errorHandler.addRetryableExceptions(
            java.net.SocketTimeoutException.class,
            org.springframework.dao.TransientDataAccessException.class
        );

        factory.setCommonErrorHandler(errorHandler);
        return factory;
    }
}
```

### 3.3 BackOff Strategies

| BackOff Type | Constructor | Behavior |
|---|---|---|
| `FixedBackOff(interval, maxAttempts)` | `new FixedBackOff(1000L, 3L)` | Retry 3 times, 1 second apart |
| `ExponentialBackOff(initialInterval, multiplier)` | `new ExponentialBackOff(1000L, 2.0)` | 1s, 2s, 4s, 8s... (default max 10 attempts) |
| `ExponentialBackOff` + `setMaxElapsedTime` | See below | Cap total retry time |
| `FixedBackOff(0, 0)` | `new FixedBackOff(0L, 0L)` | **No retries** — send to DLT immediately |

```java
// Exponential backoff capped at 30 seconds total
ExponentialBackOff backOff = new ExponentialBackOff(1000L, 2.0);
backOff.setMaxElapsedTime(30_000L);  // stop retrying after 30s total

// Fixed backoff: 3 retries, 2 seconds apart
FixedBackOff backOff = new FixedBackOff(2000L, 3L);

// No retries at all — go straight to DLT
FixedBackOff backOff = new FixedBackOff(0L, 0L);
```

> **WARNING:** Always ensure your total retry time (all attempts × backoff) is **well under** `max.poll.interval.ms` (default 5 minutes). Otherwise the consumer will be ejected from the group.
>
> **Math check:** `ExponentialBackOff(1000, 2.0)` with 10 attempts = 1 + 2 + 4 + 8 + 16 + 32 + 64 + 128 + 256 + 512 = **1023 seconds** ≈ 17 minutes 🚨. Set `maxElapsedTime` or `maxAttempts` to cap this!

### 3.4 Retry Listeners — Observing Retry Attempts

You can hook into each retry attempt for logging, metrics, or custom logic:

```java
DefaultErrorHandler errorHandler = new DefaultErrorHandler(recoverer, backOff);

errorHandler.setRetryListeners((record, ex, deliveryAttempt) -> {
    log.warn("Retry attempt {} for record: topic={}, partition={}, offset={}, exception={}",
        deliveryAttempt,
        record.topic(),
        record.partition(),
        record.offset(),
        ex.getMessage());

    // Track retry metrics
    meterRegistry.counter("kafka.consumer.retries",
        "topic", record.topic(),
        "attempt", String.valueOf(deliveryAttempt))
        .increment();
});
```

---

## 4. DeadLetterPublishingRecoverer — Deep Dive

This is the component that actually **publishes** the failed record to the DLT.

### 4.1 Default Behavior

```java
// Simplest form — uses default topic naming
new DeadLetterPublishingRecoverer(kafkaTemplate)
```

Default conventions:
- **Topic name:** `<original-topic>.DLT` (e.g., `orders` → `orders.DLT`)
- **Partition:** Same partition number as the original (if it exists in DLT), otherwise partition 0
- **Key:** Same key as the original record
- **Value:** Same value as the original record (even if it was the cause of failure)
- **Headers:** Exception info is added as Kafka headers (see Section 7)

### 4.2 Custom Destination Resolver

You can control **where** failed records go:

```java
// Route to different DLTs based on the topic or exception
DeadLetterPublishingRecoverer recoverer = new DeadLetterPublishingRecoverer(
    kafkaTemplate,
    (record, exception) -> {
        // Custom routing logic
        if (exception.getCause() instanceof DeserializationException) {
            return new TopicPartition("deserialize-errors", record.partition());
        }
        if (record.topic().equals("payments")) {
            return new TopicPartition("payments-critical-dlq", record.partition());
        }
        // Default: <topic>.DLT
        return new TopicPartition(record.topic() + ".DLT", record.partition());
    }
);
```

### 4.3 Multiple KafkaTemplates for Different DLT Types

If your DLTs use different serialization formats:

```java
// Use different KafkaTemplates for different key/value types
DeadLetterPublishingRecoverer recoverer = new DeadLetterPublishingRecoverer(
    Map.of(
        byte[].class, bytesKafkaTemplate,    // for raw byte DLTs
        String.class, stringKafkaTemplate     // for string DLTs
    ),
    (record, exception) -> new TopicPartition(record.topic() + ".DLT", record.partition())
);
```

### 4.4 Handling DLT Publish Failures

What if publishing to the DLT **itself** fails? (e.g., DLT topic doesn't exist, broker is down)

```java
DeadLetterPublishingRecoverer recoverer =
    new DeadLetterPublishingRecoverer(kafkaTemplate);

// Log the failure instead of throwing (default throws, which causes a loop)
recoverer.setFailIfSendResultIsError(true);  // default: true

// Custom handling when DLT publish fails
recoverer.setRetryTemplate(retryTemplate);   // retry publishing to DLT

// Or: use a different recovery fallback
recoverer.setSkipSameTopicFatalExceptions(false);
```

### 4.5 Ensuring DLT Topics Exist

By default, Kafka auto-creates topics if `auto.create.topics.enable=true` on the broker. But in production, you should **explicitly create** DLT topics:

```java
@Configuration
public class KafkaTopicConfig {

    @Bean
    public NewTopic ordersDlt() {
        return TopicBuilder.name("orders.DLT")
            .partitions(6)               // match original topic's partition count
            .replicas(3)
            .config(TopicConfig.RETENTION_MS_CONFIG, 
                    String.valueOf(30L * 24 * 60 * 60 * 1000))  // 30 days
            .build();
    }
}
```

> **Tip:** Give DLTs a **longer retention** than the original topic. You need time to investigate and reprocess.

### 4.6 Verifying Destination Partition Exists

```java
DeadLetterPublishingRecoverer recoverer =
    new DeadLetterPublishingRecoverer(kafkaTemplate);

// If the DLT has fewer partitions than the source, avoid IllegalArgumentException
recoverer.setVerifyPartition(true);
// With this enabled, if partition 5 doesn't exist in the DLT,
// it will fall back to partition 0 instead of failing
```

---

## 5. Non-Blocking Retries — `@RetryableTopic` (Spring Kafka 2.9+)

This is the more **advanced, production-grade** approach. Instead of blocking the consumer thread during retries, failed messages are published to **intermediate retry topics** and consumed with a delay.

### 5.1 Why Non-Blocking?

```
BLOCKING RETRY PROBLEM:

  Partition 0: [msg-40] [msg-41] [msg-42(poison)] [msg-43] [msg-44] ...
                                      ↑
                         Consumer stuck here for N × backoff seconds
                         msg-43 and msg-44 are BLOCKED

NON-BLOCKING RETRY SOLUTION:

  Partition 0:  [msg-40] [msg-41] [msg-42] [msg-43] [msg-44] ...
                                     │        ↑
                                     │   Processed immediately!
                                     ▼
                          orders-retry-0: [msg-42]  (retry after 1s)
                                              │
                                              ▼ (still fails)
                          orders-retry-1: [msg-42]  (retry after 2s)
                                              │
                                              ▼ (still fails)
                          orders-retry-2: [msg-42]  (retry after 4s)
                                              │
                                              ▼ (still fails)
                          orders.DLT:     [msg-42]  (all retries exhausted)
```

### 5.2 Basic Usage with Annotation

```java
@Component
public class OrderConsumer {

    @RetryableTopic(
        attempts = "4",                    // 1 original + 3 retries
        backoff = @Backoff(
            delay = 1000,                  // 1 second initial delay
            multiplier = 2.0,             // exponential: 1s, 2s, 4s
            maxDelay = 10000              // cap at 10 seconds
        ),
        autoCreateTopics = "true",         // auto-create retry + DLT topics
        topicSuffixingStrategy = TopicSuffixingStrategy.SUFFIX_WITH_INDEX_VALUE,
        dltStrategy = DltStrategy.FAIL_ON_ERROR    // what if DLT publish fails
    )
    @KafkaListener(topics = "orders", groupId = "order-service")
    public void listen(ConsumerRecord<String, OrderEvent> record) {
        processOrder(record.value());  // throws exception on failure
    }

    // DLT handler method — processes messages that land in orders.DLT
    @DltHandler
    public void handleDlt(ConsumerRecord<String, OrderEvent> record) {
        log.error("DLT Received: topic={}, key={}, value={}",
            record.topic(), record.key(), record.value());
        // Alert, persist to DB, or schedule for manual review
    }
}
```

### 5.3 Topic Topology Created

With the above config, Spring automatically creates:

```
orders                    ← original topic (you consume from here)
orders-retry-0            ← 1st retry (1 second delay)
orders-retry-1            ← 2nd retry (2 second delay)
orders-retry-2            ← 3rd retry (4 second delay)
orders.DLT                ← dead letter topic (all retries exhausted)
```

### 5.4 How Delays Work Internally

Spring Kafka uses the **consumer's poll loop** to implement delays — it does NOT use `Thread.sleep()`:

```
1. Failed record published to retry topic with a backoff timestamp header
2. Retry topic consumer polls the record
3. Consumer checks: current_time >= backoff_timestamp ?
   ├── YES → process the record (retry)
   └── NO  → pause the consumer partition, poll again later
              (the partition is paused so other records aren't fetched)
```

This means retry topic consumers are **not sleeping**, they're actively polling but waiting for the right time.

### 5.5 @RetryableTopic Full Options

```java
@RetryableTopic(
    // Number of attempts (including the first one)
    attempts = "4",

    // Backoff configuration
    backoff = @Backoff(
        delay = 1000,           // ms before first retry
        multiplier = 2.0,       // exponential multiplier
        maxDelay = 60000        // max delay cap (ms)
    ),

    // Which exceptions to retry vs send straight to DLT
    include = {TransientException.class, TimeoutException.class},
    exclude = {DeserializationException.class, ValidationException.class},

    // How to handle traversal across topics
    // REUSE_RETRY_TOPIC: all retries go to one retry topic
    // SUFFIX_WITH_INDEX_VALUE: orders-retry-0, orders-retry-1, etc.
    topicSuffixingStrategy = TopicSuffixingStrategy.SUFFIX_WITH_INDEX_VALUE,

    // What to do when DLT fails
    // ALWAYS_RETRY_ON_ERROR: retry the DLT publish
    // FAIL_ON_ERROR: fail (stops the container)
    // NO_DLT: don't use a DLT at all (discard after retries)
    dltStrategy = DltStrategy.ALWAYS_RETRY_ON_ERROR,

    // Retry topic naming — default suffix
    retryTopicSuffix = "-retry",
    dltTopicSuffix = ".DLT",

    // Auto-create retry + DLT topics
    autoCreateTopics = "true",
    numPartitions = "3",
    replicationFactor = "3",

    // Fix a specific retry pattern for all topics
    fixedDelayTopicStrategy = FixedDelayStrategy.SINGLE_TOPIC,

    // Timeout — max time to keep retrying (ms)
    timeout = "300000"   // 5 minutes max
)
```

### 5.6 SUFFIX_WITH_INDEX_VALUE vs SINGLE_TOPIC

| Strategy | Topics Created | Use Case |
|---|---|---|
| `SUFFIX_WITH_INDEX_VALUE` | `orders-retry-0`, `orders-retry-1`, `orders-retry-2` | Different delays per retry level |
| `SINGLE_TOPIC` (with `fixedDelayTopicStrategy`) | `orders-retry` (single topic) | Same delay for all retries; simpler topology |

### 5.7 DltStrategy Options

| Strategy | Behavior |
|---|---|
| `ALWAYS_RETRY_ON_ERROR` | If DLT publish fails, keep retrying. **Safest.** |
| `FAIL_ON_ERROR` | If DLT publish fails, stop the container. Requires manual intervention. |
| `NO_DLT` | Don't use a DLT at all. After retries are exhausted, the message is discarded. ⚠️ |

### 5.8 Programmatic Configuration (Alternative to Annotations)

```java
@Configuration
public class RetryTopicConfig {

    @Bean
    public RetryTopicConfiguration orderRetryConfig(
            KafkaTemplate<String, Object> kafkaTemplate) {

        return RetryTopicConfigurationBuilder
            .newInstance()
            .exponentialBackoff(1000, 2.0, 30000)   // 1s, 2s, 4s... max 30s
            .maxAttempts(5)
            .includeTopics("orders", "payments")     // apply to these topics only
            .retryOn(TransientException.class)
            .notRetryOn(DeserializationException.class)
            .setTopicSuffixingStrategy(
                TopicSuffixingStrategy.SUFFIX_WITH_INDEX_VALUE)
            .dltHandlerMethod("orderDltHandler", "handleDlt")
            .create(kafkaTemplate);
    }

    @Component("orderDltHandler")
    public static class OrderDltHandler {
        public void handleDlt(ConsumerRecord<String, Object> record) {
            log.error("DLT: {}", record.value());
        }
    }
}
```

### 5.9 Global Retry Topic Configuration

Apply non-blocking retries to **all** `@KafkaListener` methods:

```java
@Configuration
public class GlobalRetryConfig {

    @Bean
    public RetryTopicConfiguration globalRetryConfig(
            KafkaTemplate<String, Object> kafkaTemplate) {

        return RetryTopicConfigurationBuilder
            .newInstance()
            .fixedBackOff(3000)           // 3 seconds between retries
            .maxAttempts(3)
            .notRetryOn(DeserializationException.class)
            .create(kafkaTemplate);
        // No .includeTopics() → applies to ALL listeners
    }
}
```

---

## 6. Exception Classification — Retryable vs Non-Retryable

Not all exceptions should be retried. Retrying a **schema error** 10 times won't magically fix it.

### 6.1 The Classification Model

```
Exception occurs
      │
      ▼
  Is it classified as NOT retryable?
      │
 YES ─┼──▶ Skip ALL retries → send directly to DLT
      │
  NO ─┼──▶ Retry using configured BackOff
      │         │
      │    All retries exhausted?
      │         │
      │    YES ─┼──▶ Send to DLT
      │    NO  ─┼──▶ Retry again
```

### 6.2 Default Non-Retryable Exceptions (Spring Kafka)

Spring Kafka ships with a default list of fatal (non-retryable) exceptions:

| Exception | Why It's Fatal |
|---|---|
| `DeserializationException` | Malformed data — retrying won't fix the bytes |
| `MessageConversionException` | Type mismatch — structural problem |
| `ConversionException` | Spring conversion failure |
| `MethodArgumentResolutionException` | Wrong method signature match |
| `NoSuchMethodException` | Listener method not found |
| `ClassCastException` | Type casting failure |

### 6.3 Customizing Classification

```java
// With DefaultErrorHandler
DefaultErrorHandler errorHandler = new DefaultErrorHandler(recoverer, backOff);

// Add custom non-retryable exceptions
errorHandler.addNotRetryableExceptions(
    com.example.InvalidOrderException.class,       // business validation failure
    com.fasterxml.jackson.core.JsonParseException.class  // bad JSON
);

// Add custom retryable exceptions (overrides built-in fatal list)
errorHandler.addRetryableExceptions(
    java.sql.SQLTransientException.class,   // DB temporarily unavailable
    java.net.ConnectException.class         // downstream service down
);
```

```java
// With @RetryableTopic — specify via annotation
@RetryableTopic(
    include = {SQLTransientException.class, ConnectException.class},  // ONLY retry these
    exclude = {InvalidOrderException.class}   // NEVER retry these
)
```

> **Rule of thumb for classification:**
> - **Retryable:** Network errors, timeouts, transient DB issues, downstream service unavailable
> - **Non-retryable:** Bad data format, schema errors, validation failures, null pointers, business rule violations

### 6.4 Exception Unwrapping

Spring Kafka automatically **unwraps** `ListenerExecutionFailedException` to examine the underlying cause. When your `@KafkaListener` throws an exception, Spring wraps it in `ListenerExecutionFailedException`. The error handler looks at the **cause**, not the wrapper, when classifying.

```
Your code throws: new InvalidOrderException("bad price")
     ↓
Spring wraps it: ListenerExecutionFailedException(cause: InvalidOrderException)
     ↓
DefaultErrorHandler checks: cause is InvalidOrderException → NOT retryable
     ↓
Skips retries → sends to DLT
```

---

## 7. DLT Headers — Metadata on Failed Records

When a message is published to the DLT, Spring Kafka attaches rich metadata as **Kafka headers**. This is essential for debugging and reprocessing.

### 7.1 Headers Added by DeadLetterPublishingRecoverer

| Header Key | Type | Value |
|---|---|---|
| `kafka_dlt-exception-fqcn` | String | Fully-qualified exception class name |
| `kafka_dlt-exception-cause-fqcn` | String | Fully-qualified cause exception class name |
| `kafka_dlt-exception-message` | String | Exception message (truncated to `maxHeaderSize`) |
| `kafka_dlt-exception-stacktrace` | String | Full stack trace (truncated to `maxHeaderSize`) |
| `kafka_dlt-original-topic` | String | Original topic name |
| `kafka_dlt-original-partition` | int | Original partition number |
| `kafka_dlt-original-offset` | long | Original offset |
| `kafka_dlt-original-timestamp` | long | Original record timestamp |
| `kafka_dlt-original-timestamp-type` | String | `CREATE_TIME` or `LOG_APPEND_TIME` |
| `kafka_dlt-original-consumer-group` | String | Consumer group that failed |

### 7.2 Accessing Headers in Your DLT Consumer

```java
@DltHandler
public void handleDlt(
        ConsumerRecord<String, OrderEvent> record,
        @Header(KafkaHeaders.DLT_EXCEPTION_FQCN) String exceptionClass,
        @Header(KafkaHeaders.DLT_EXCEPTION_MESSAGE) String exceptionMessage,
        @Header(KafkaHeaders.DLT_ORIGINAL_TOPIC) String originalTopic,
        @Header(KafkaHeaders.DLT_ORIGINAL_OFFSET) byte[] originalOffset,
        @Header(KafkaHeaders.DLT_EXCEPTION_STACKTRACE) String stacktrace) {

    log.error("""
        DLT Record Received:
          Original Topic: {}
          Exception: {} — {}
          Key: {}
          Value: {}
        """,
        originalTopic, exceptionClass, exceptionMessage,
        record.key(), record.value());

    // Persist to a database for manual review
    failedMessageRepository.save(new FailedMessage(
        originalTopic,
        record.key(),
        record.value().toString(),
        exceptionClass,
        exceptionMessage,
        stacktrace
    ));
}
```

### 7.3 Adding Custom Headers

```java
DeadLetterPublishingRecoverer recoverer =
    new DeadLetterPublishingRecoverer(kafkaTemplate);

// Add custom headers alongside the default ones
recoverer.setHeadersFunction((record, ex) -> {
    Headers headers = new RecordHeaders();
    headers.add("failed-service", "order-service".getBytes());
    headers.add("failed-timestamp",
        String.valueOf(System.currentTimeMillis()).getBytes());
    headers.add("environment", "production".getBytes());
    return headers;
});
```

### 7.4 Accessing Headers in a Standalone DLT Consumer (without @DltHandler)

If you have a separate `@KafkaListener` on the DLT topic (not using `@DltHandler`):

```java
@KafkaListener(topics = "orders.DLT", groupId = "dlt-processor")
public void handleDlt(ConsumerRecord<String, String> record) {
    // Access headers manually from the ConsumerRecord
    Header exceptionHeader = record.headers().lastHeader(
        KafkaHeaders.DLT_EXCEPTION_FQCN);
    Header originalTopicHeader = record.headers().lastHeader(
        KafkaHeaders.DLT_ORIGINAL_TOPIC);

    String exceptionClass = exceptionHeader != null
        ? new String(exceptionHeader.value()) : "unknown";
    String originalTopic = originalTopicHeader != null
        ? new String(originalTopicHeader.value()) : "unknown";

    log.error("DLT from [{}]: exception={}, key={}, value={}",
        originalTopic, exceptionClass, record.key(), record.value());
}
```

---

## 8. DLT Consumer Patterns — What to Do with Dead Letters

Once a message lands in the DLT, you need a strategy for dealing with it. Here are four production-proven patterns:

### Pattern 1: Log and Alert

The simplest approach — log the failure and notify someone.

```java
@Component
public class DltConsumer {

    @KafkaListener(topics = "orders.DLT", groupId = "dlt-processor")
    public void handleDlt(ConsumerRecord<String, String> record) {
        log.error("DLT message from [{}]: key={}, value={}",
            record.topic(), record.key(), record.value());

        // Send alert to Slack/PagerDuty
        alertService.sendAlert(
            "Dead letter received for key: " + record.key(),
            AlertSeverity.HIGH
        );
    }
}
```

### Pattern 2: Persist to Database for Manual Review

Store the failed message in a database so a support team can investigate:

```java
@KafkaListener(topics = "orders.DLT", groupId = "dlt-processor")
public void handleDlt(ConsumerRecord<String, String> record,
                       @Header(KafkaHeaders.DLT_EXCEPTION_MESSAGE) String error) {

    FailedMessage failed = FailedMessage.builder()
        .originalTopic(new String(record.headers()
            .lastHeader(KafkaHeaders.DLT_ORIGINAL_TOPIC).value()))
        .messageKey(record.key())
        .messageValue(record.value())
        .errorMessage(error)
        .failedAt(Instant.now())
        .status(FailedMessageStatus.PENDING)
        .build();

    failedMessageRepository.save(failed);
}
```

### Pattern 3: Automated Retry with Scheduled Job

Periodically re-attempt processing of failed messages:

```java
@Service
public class DltReprocessingService {

    @Autowired
    private KafkaTemplate<String, String> kafkaTemplate;

    @Autowired
    private FailedMessageRepository failedMessageRepository;

    // Reprocess eligible DLT messages every hour
    @Scheduled(fixedRate = 3600000)
    public void reprocessFailedMessages() {
        List<FailedMessage> pending = failedMessageRepository
            .findByStatusAndRetryCountLessThan(
                FailedMessageStatus.PENDING, 5);

        for (FailedMessage msg : pending) {
            try {
                kafkaTemplate.send(msg.getOriginalTopic(),
                    msg.getMessageKey(), msg.getMessageValue()).get();

                msg.setStatus(FailedMessageStatus.REPROCESSED);
            } catch (Exception e) {
                msg.setRetryCount(msg.getRetryCount() + 1);
                if (msg.getRetryCount() >= 5) {
                    msg.setStatus(FailedMessageStatus.ABANDONED);
                }
            }
            failedMessageRepository.save(msg);
        }
    }
}
```

### Pattern 4: REST API for Manual Reprocessing

Expose the DLT failure records via an admin API:

```java
@RestController
@RequestMapping("/api/dlt")
public class DltController {

    @Autowired private FailedMessageRepository repo;
    @Autowired private KafkaTemplate<String, String> kafkaTemplate;

    @GetMapping
    public List<FailedMessage> listFailed(
            @RequestParam(defaultValue = "PENDING") String status) {
        return repo.findByStatus(FailedMessageStatus.valueOf(status));
    }

    @PostMapping("/{id}/reprocess")
    public ResponseEntity<String> reprocess(@PathVariable Long id) {
        FailedMessage msg = repo.findById(id)
            .orElseThrow(() -> new ResponseStatusException(HttpStatus.NOT_FOUND));

        kafkaTemplate.send(msg.getOriginalTopic(),
            msg.getMessageKey(), msg.getMessageValue());

        msg.setStatus(FailedMessageStatus.REPROCESSED);
        repo.save(msg);

        return ResponseEntity.ok("Requeued to " + msg.getOriginalTopic());
    }

    @PostMapping("/reprocess-all")
    public ResponseEntity<String> reprocessAll() {
        List<FailedMessage> pending =
            repo.findByStatus(FailedMessageStatus.PENDING);
        pending.forEach(msg -> {
            kafkaTemplate.send(msg.getOriginalTopic(),
                msg.getMessageKey(), msg.getMessageValue());
            msg.setStatus(FailedMessageStatus.REPROCESSED);
            repo.save(msg);
        });
        return ResponseEntity.ok("Requeued " + pending.size() + " messages");
    }
}
```

### Pattern Comparison

| Pattern | Effort | Automation | Best For |
|---|---|---|---|
| Log & Alert | Low | None | Dev/staging, low-volume systems |
| DB + Manual Review | Medium | None | Production, compliance-heavy domains |
| Scheduled Reprocess | Medium | Full | Transient failures (DB down, service restart) |
| REST API | High | On-demand | Ops teams, dashboards, fine-grained control |

> **Recommendation:** Use **Pattern 2 + 4** (DB persist + REST API) for production. This gives you both a safety net and the ability to selectively reprocess.

---

## 9. Deserialization Errors — A Special Case

Deserialization errors are **unique** because the exception happens **before** your `@KafkaListener` method is even called. The record's value is unparseable, so your listener never sees it.

### 9.1 The Problem

```
Broker sends bytes → Deserializer fails → KafkaConsumer throws
                                              │
                             Your @KafkaListener is NEVER called
                                              │
                                   ErrorHandler must handle it
```

Without proper handling, a single bad message causes the consumer to crash and restart in an infinite loop — the so-called **poison pill** problem.

### 9.2 Solution: ErrorHandlingDeserializer

Spring Kafka provides a wrapper deserializer that catches deserialization failures and passes them as a special header instead of throwing:

```yaml
spring:
  kafka:
    consumer:
      key-deserializer: org.springframework.kafka.support.serializer.ErrorHandlingDeserializer
      value-deserializer: org.springframework.kafka.support.serializer.ErrorHandlingDeserializer
      properties:
        spring.deserializer.key.delegate.class: org.apache.kafka.common.serialization.StringDeserializer
        spring.deserializer.value.delegate.class: org.springframework.kafka.support.serializer.JsonDeserializer
        spring.json.trusted.packages: "com.example.dto"
```

### 9.3 How It Works Internally

```
Bytes arrive from broker
       │
       ▼
ErrorHandlingDeserializer
       │
       ├── Delegate to actual deserializer (JsonDeserializer)
       │        │
       │   Success? → return deserialized object
       │        │
       │   Failure? → set the failed bytes as a header:
       │              "springDeserializerExceptionValue" → the raw bytes
       │              Return null as the value
       │
       ▼
 @KafkaListener receives the record (value=null, exception in header)
       │
       ▼
 DefaultErrorHandler sees it's a DeserializationException
       │
       ▼
 NOT RETRYABLE (by default) → sent directly to DLT
```

> **Key insight:** With `ErrorHandlingDeserializer`, even completely unreadable messages are gracefully routed to the DLT instead of causing an infinite loop or crashing the consumer.

### 9.4 Java Config Equivalent

```java
@Bean
public ConsumerFactory<String, Object> consumerFactory() {
    Map<String, Object> props = new HashMap<>();
    props.put(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, "localhost:9092");

    // Wrap with ErrorHandlingDeserializer
    props.put(ConsumerConfig.KEY_DESERIALIZER_CLASS_CONFIG,
        ErrorHandlingDeserializer.class);
    props.put(ConsumerConfig.VALUE_DESERIALIZER_CLASS_CONFIG,
        ErrorHandlingDeserializer.class);

    // Set actual deserializers as delegates
    props.put(ErrorHandlingDeserializer.KEY_DESERIALIZER_CLASS,
        StringDeserializer.class);
    props.put(ErrorHandlingDeserializer.VALUE_DESERIALIZER_CLASS,
        JsonDeserializer.class);

    props.put(JsonDeserializer.TRUSTED_PACKAGES, "com.example.dto");

    return new DefaultKafkaConsumerFactory<>(props);
}
```

---

## 10. Complete Production-Ready Example

Here is a full, production-ready error handling setup combining everything:

### 10.1 Configuration

```java
@Configuration
@EnableKafka
public class KafkaConsumerConfig {

    @Value("${spring.kafka.bootstrap-servers}")
    private String bootstrapServers;

    @Autowired
    private KafkaTemplate<String, Object> kafkaTemplate;

    // ── Consumer Factory ──
    @Bean
    public ConsumerFactory<String, Object> consumerFactory() {
        Map<String, Object> props = new HashMap<>();
        props.put(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, bootstrapServers);
        props.put(ConsumerConfig.GROUP_ID_CONFIG, "order-service");
        props.put(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG, false);
        props.put(ConsumerConfig.AUTO_OFFSET_RESET_CONFIG, "earliest");

        // Use ErrorHandlingDeserializer to catch deserialization errors
        props.put(ConsumerConfig.KEY_DESERIALIZER_CLASS_CONFIG,
            ErrorHandlingDeserializer.class);
        props.put(ConsumerConfig.VALUE_DESERIALIZER_CLASS_CONFIG,
            ErrorHandlingDeserializer.class);
        props.put(ErrorHandlingDeserializer.KEY_DESERIALIZER_CLASS,
            StringDeserializer.class);
        props.put(ErrorHandlingDeserializer.VALUE_DESERIALIZER_CLASS,
            JsonDeserializer.class);
        props.put(JsonDeserializer.TRUSTED_PACKAGES, "com.example.dto");

        // Consumer tuning
        props.put(ConsumerConfig.MAX_POLL_RECORDS_CONFIG, 100);
        props.put(ConsumerConfig.MAX_POLL_INTERVAL_MS_CONFIG, 300000);

        return new DefaultKafkaConsumerFactory<>(props);
    }

    // ── Listener Container Factory with Error Handling ──
    @Bean
    public ConcurrentKafkaListenerContainerFactory<String, Object>
            kafkaListenerContainerFactory() {

        ConcurrentKafkaListenerContainerFactory<String, Object> factory =
            new ConcurrentKafkaListenerContainerFactory<>();
        factory.setConsumerFactory(consumerFactory());
        factory.setConcurrency(3);
        factory.getContainerProperties().setAckMode(
            ContainerProperties.AckMode.RECORD);

        // ── Error Handler ──
        DefaultErrorHandler errorHandler = new DefaultErrorHandler(
            deadLetterPublishingRecoverer(),
            exponentialBackOff()
        );

        // Classify exceptions
        errorHandler.addNotRetryableExceptions(
            DeserializationException.class,
            MessageConversionException.class,
            com.example.exceptions.InvalidOrderException.class
        );

        // Custom retry listener — observe every retry attempt
        errorHandler.setRetryListeners((record, ex, deliveryAttempt) -> {
            log.warn("Retry attempt {} for record: topic={}, offset={}, exception={}",
                deliveryAttempt, record.topic(), record.offset(),
                ex.getMessage());
        });

        factory.setCommonErrorHandler(errorHandler);
        return factory;
    }

    // ── DLT Recoverer ──
    @Bean
    public DeadLetterPublishingRecoverer deadLetterPublishingRecoverer() {
        DeadLetterPublishingRecoverer recoverer =
            new DeadLetterPublishingRecoverer(kafkaTemplate,
                (record, ex) -> {
                    // Route deserialization errors to a special topic
                    if (ex.getCause() instanceof DeserializationException) {
                        return new TopicPartition(
                            "deserialize-errors", record.partition());
                    }
                    return new TopicPartition(
                        record.topic() + ".DLT", record.partition());
                });

        // Verify destination topic exists
        recoverer.setVerifyPartition(true);
        return recoverer;
    }

    // ── BackOff Strategy ──
    @Bean
    public ExponentialBackOff exponentialBackOff() {
        ExponentialBackOff backOff = new ExponentialBackOff(1000L, 2.0);
        backOff.setMaxElapsedTime(30_000L);  // max 30 seconds of retrying
        return backOff;
    }

    // ── Ensure DLT topics exist ──
    @Bean
    public NewTopic ordersDlt() {
        return TopicBuilder.name("orders.DLT")
            .partitions(6)
            .replicas(3)
            .config(TopicConfig.RETENTION_MS_CONFIG, "2592000000")  // 30 days
            .build();
    }

    @Bean
    public NewTopic deserializeErrorsTopic() {
        return TopicBuilder.name("deserialize-errors")
            .partitions(3)
            .replicas(3)
            .build();
    }
}
```

### 10.2 Listener with DLT Handler

```java
@Component
@Slf4j
public class OrderConsumer {

    @Autowired
    private OrderService orderService;

    @KafkaListener(topics = "orders", groupId = "order-service")
    public void listen(ConsumerRecord<String, OrderEvent> record,
                        Acknowledgment ack) {
        log.info("Processing order: key={}, partition={}, offset={}",
            record.key(), record.partition(), record.offset());

        orderService.processOrder(record.value());  // may throw
        ack.acknowledge();
    }
}

@Component
@Slf4j
public class DltProcessor {

    @Autowired
    private FailedMessageRepository failedMessageRepository;

    @Autowired
    private AlertService alertService;

    @KafkaListener(topics = "orders.DLT", groupId = "dlt-processor")
    public void handleOrdersDlt(
            ConsumerRecord<String, String> record,
            @Header(KafkaHeaders.DLT_EXCEPTION_FQCN) String exceptionType,
            @Header(KafkaHeaders.DLT_EXCEPTION_MESSAGE) String errorMsg,
            @Header(KafkaHeaders.DLT_ORIGINAL_TOPIC) String originalTopic,
            @Header(KafkaHeaders.DLT_ORIGINAL_OFFSET) byte[] originalOffset) {

        log.error("DLT received: topic={}, key={}, error={}: {}",
            originalTopic, record.key(), exceptionType, errorMsg);

        // 1. Persist for manual review
        failedMessageRepository.save(FailedMessage.builder()
            .originalTopic(originalTopic)
            .messageKey(record.key())
            .messageValue(record.value())
            .exceptionType(exceptionType)
            .errorMessage(errorMsg)
            .receivedAt(Instant.now())
            .build());

        // 2. Alert the team
        alertService.sendCriticalAlert(
            String.format("Order DLT: key=%s, error=%s",
                record.key(), errorMsg));
    }
}
```

---

## 11. Non-Blocking Retry with @RetryableTopic — Full Example

```java
@Component
@Slf4j
public class PaymentConsumer {

    @RetryableTopic(
        attempts = "4",
        backoff = @Backoff(delay = 5000, multiplier = 3.0, maxDelay = 60000),
        include = {PaymentGatewayException.class, TimeoutException.class},
        exclude = {InvalidPaymentException.class},
        autoCreateTopics = "true",
        numPartitions = "3",
        replicationFactor = "3",
        topicSuffixingStrategy = TopicSuffixingStrategy.SUFFIX_WITH_INDEX_VALUE,
        dltStrategy = DltStrategy.ALWAYS_RETRY_ON_ERROR
    )
    @KafkaListener(topics = "payments", groupId = "payment-service")
    public void processPayment(ConsumerRecord<String, PaymentEvent> record) {
        log.info("Processing payment: {}", record.key());
        paymentService.charge(record.value());  // may throw
    }

    @DltHandler
    public void handlePaymentDlt(ConsumerRecord<String, PaymentEvent> record,
                                  @Header(KafkaHeaders.DLT_EXCEPTION_MESSAGE)
                                      String error) {
        log.error("Payment permanently failed: key={}, error={}",
            record.key(), error);

        // Refund the customer, alert support, etc.
        paymentService.markAsFailed(record.value());
        notificationService.notifyCustomer(record.value().getCustomerId(),
            "Your payment could not be processed. Please contact support.");
    }
}
```

**Topics created automatically:**

```
payments                    ← original (you consume from here)
payments-retry-0            ← retry after 5s
payments-retry-1            ← retry after 15s
payments-retry-2            ← retry after 45s (capped at 60s)
payments.DLT                ← all 4 attempts exhausted
```

---

## 12. Blocking vs Non-Blocking — Decision Guide

```
┌─────────────────────────────────────────────────────────────────┐
│              Which retry strategy should I use?                   │
├──────────────────────┬──────────────────────────────────────────┤
│                      │                                          │
│  Use BLOCKING        │  Use NON-BLOCKING                        │
│  (DefaultErrorHandler│  (@RetryableTopic)                       │
│                      │                                          │
│  ✓ Simple setup      │  ✓ High-throughput systems               │
│  ✓ Ordering matters  │  ✓ Long retry delays needed (minutes+)   │
│  ✓ Fast retries      │  ✓ Can't afford to block partitions      │
│    (< 30 seconds     │  ✓ Independent record processing         │
│     total)           │  ✓ Ordering per-record NOT critical       │
│  ✓ Few partitions    │  ✓ Many partitions & consumers            │
│                      │                                          │
│  ✗ Blocks partition  │  ✗ More complex topic topology            │
│  ✗ Consumer lag risk │  ✗ More topics to monitor & manage        │
│                      │  ✗ Record ordering not preserved          │
└──────────────────────┴──────────────────────────────────────────┘
```

> **Our recommendation:** Start with `DefaultErrorHandler` + DLT (blocking). Switch to `@RetryableTopic` (non-blocking) only when you observe partition blocking as a real problem in production, or when your retry delays are longer than 30 seconds.

---

## 13. DLT Monitoring & Metrics

### 13.1 Key Metrics to Track

```yaml
# application.yml — expose Kafka metrics
management:
  endpoints:
    web:
      exposure:
        include: health,metrics,prometheus
  metrics:
    tags:
      application: order-service
```

| Metric | What It Tells You |
|---|---|
| `spring.kafka.listener.seconds` | Time spent processing records |
| `kafka.consumer.records.consumed.total` on DLT topic | Number of DLT records (should ideally be 0) |
| Custom counter for DLT entries | Track via Micrometer |

### 13.2 Custom DLT Counter with Micrometer

```java
@Component
public class DltMetrics {

    private final Counter dltCounter;

    public DltMetrics(MeterRegistry registry) {
        this.dltCounter = Counter.builder("kafka.dlt.messages")
            .description("Number of messages sent to DLT")
            .tag("topic", "orders")
            .register(registry);
    }

    @KafkaListener(topics = "orders.DLT", groupId = "dlt-metrics")
    public void countDltMessages(ConsumerRecord<String, ?> record) {
        dltCounter.increment();
    }
}
```

### 13.3 Alerting Rules (Example for Prometheus / Grafana)

```yaml
# Prometheus alerting rule
groups:
  - name: kafka-dlt-alerts
    rules:
      - alert: DLTMessagesDetected
        expr: increase(kafka_dlt_messages_total[5m]) > 0
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Dead letter messages detected"
          description: "{{ $value }} messages sent to DLT in the last 5 minutes"

      - alert: DLTMessagesSpiking
        expr: rate(kafka_dlt_messages_total[5m]) > 10
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "DLT message rate is spiking"
```

---

## 14. Quick Reference — Error Handling Cheat Sheet

| Want to... | Use |
|---|---|
| Retry N times then DLT | `DefaultErrorHandler` + `DeadLetterPublishingRecoverer` + `FixedBackOff` |
| Retry with exponential backoff then DLT | `DefaultErrorHandler` + `DeadLetterPublishingRecoverer` + `ExponentialBackOff` |
| Skip retries, go straight to DLT | `DefaultErrorHandler` + `FixedBackOff(0, 0)` |
| Non-blocking retries with DLT | `@RetryableTopic` + `@DltHandler` |
| Handle deserialization errors | `ErrorHandlingDeserializer` wrapping your real deserializer |
| Custom DLT topic name | Custom `BiFunction` in `DeadLetterPublishingRecoverer` constructor |
| Log every retry attempt | `errorHandler.setRetryListeners(...)` |
| Don't retry certain exceptions | `errorHandler.addNotRetryableExceptions(...)` |
| Monitor DLT message count | Micrometer `Counter` on DLT consumer |
| Reprocess DLT messages | Consume from DLT, republish to original topic |
| Apply retries to all listeners globally | `RetryTopicConfigurationBuilder` without `.includeTopics()` |
| Use different retry delays per topic | Multiple `RetryTopicConfiguration` beans with `.includeTopics()` |

---

## 15. Common Mistakes

| Mistake | Consequence | Fix |
|---|---|---|
| Total retry time > `max.poll.interval.ms` | Consumer ejected from group, rebalance storm | Set `maxElapsedTime` on backoff, or increase `max.poll.interval.ms` |
| Not using `ErrorHandlingDeserializer` | Bad messages cause infinite consumer restart loop | Always wrap deserializers with `ErrorHandlingDeserializer` |
| DLT topic doesn't exist + `auto.create.topics.enable=false` | DLT publish fails, record is lost | Pre-create DLT topics with `NewTopic` beans |
| Same `group.id` for DLT consumer and main consumer | Weird partition assignment issues | Use a different `groupId` for DLT consumers |
| Not monitoring DLT topics | Silent data loss — nobody notices messages going to DLT | Set up alerts on DLT message count |
| Retrying non-retryable exceptions | Wasted time and resources for errors that will never self-heal | Classify exceptions properly with `addNotRetryableExceptions` |
| Infinite retries with no DLT | Partition permanently blocked | Always configure a recoverer or max attempts |
| Using `@RetryableTopic` and expecting ordering | Messages retry out of order | Use blocking retries if ordering matters |
| Forgetting `@DltHandler` with `@RetryableTopic` | DLT messages silently consumed with default no-op handler | Always define a `@DltHandler` method |

---

> **Summary:** DLT error handling in Spring Kafka is a multi-layered system: `DefaultErrorHandler` provides blocking retries with configurable backoff; `@RetryableTopic` enables non-blocking retries via intermediate retry topics; `DeadLetterPublishingRecoverer` publishes failed records to DLT topics with rich exception metadata in headers; and `ErrorHandlingDeserializer` catches deserialization failures before they reach your listener. For production: classify your exceptions, monitor your DLTs, pre-create topics, and always have a reprocessing strategy.
