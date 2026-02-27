# Spring Boot Tutorial — Part 1: Fundamentals

## What is Spring Boot?

Spring Boot is an **opinionated framework** built on top of the Spring Framework. It removes boilerplate configuration and lets you build production-ready apps fast.

**Spring vs Spring Boot:**
| Feature | Spring | Spring Boot |
|---|---|---|
| Configuration | Manual XML / Java Config | Auto-configuration |
| Server | External (Tomcat WAR deploy) | Embedded server (just run JAR) |
| Dependencies | Manual version management | Starter POMs handle it |
| Setup time | Hours | Minutes |

---

## 1. Setting Up Your First Project

### Using Spring Initializr (Recommended)

Go to [start.spring.io](https://start.spring.io) and select:

- **Project:** Maven (or Gradle)
- **Language:** Java
- **Spring Boot:** Latest stable (3.x)
- **Group:** `com.example`
- **Artifact:** `demo`
- **Packaging:** Jar
- **Java:** 17 or 21

**Essential Starter Dependencies:**

| Starter | What it gives you |
|---|---|
| `spring-boot-starter-web` | Embedded Tomcat, Spring MVC, REST support |
| `spring-boot-starter-data-jpa` | Hibernate + Spring Data JPA |
| `spring-boot-starter-thymeleaf` | Server-side HTML templating |
| `spring-boot-starter-validation` | Bean Validation (JSR 380) |
| `spring-boot-starter-security` | Authentication & authorization |
| `spring-boot-starter-test` | JUnit 5, Mockito, Spring Test |
| `spring-boot-devtools` | Hot reload during development |

### Project Structure

```
src/
├── main/
│   ├── java/com/example/demo/
│   │   ├── DemoApplication.java          ← Entry point
│   │   ├── controller/                   ← REST / MVC controllers
│   │   ├── service/                      ← Business logic
│   │   ├── repository/                   ← Data access (JPA repos)
│   │   ├── model/                        ← Entity classes
│   │   ├── dto/                          ← Data Transfer Objects
│   │   ├── config/                       ← Configuration classes
│   │   └── exception/                    ← Custom exceptions
│   └── resources/
│       ├── application.properties        ← App configuration
│       ├── application.yml               ← Alternative YAML config
│       ├── static/                       ← CSS, JS, images
│       └── templates/                    ← Thymeleaf HTML templates
└── test/
    └── java/com/example/demo/           ← Test classes
```

---

## 2. The Entry Point — `@SpringBootApplication`

```java
package com.example.demo;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication  // ← This single annotation does THREE things
public class DemoApplication {
    public static void main(String[] args) {
        SpringApplication.run(DemoApplication.class, args);
    }
}
```

### What `@SpringBootApplication` actually is:

```java
@SpringBootConfiguration   // → Marks this as a configuration class (like @Configuration)
@EnableAutoConfiguration   // → Tells Spring Boot to auto-configure beans based on classpath
@ComponentScan             // → Scans THIS package + sub-packages for @Component classes
```

> **⚠️ CRITICAL TIP:** Place your main class in the **root package** (e.g., `com.example.demo`). Component scanning only scans the main class's package and below. If your controllers are in `com.other.package`, they **won't be found**.

---

## 3. Dependency Injection (DI) — The Core of Spring

Spring manages object creation for you. You declare **what** you need; Spring provides **how**.

### The IoC Container

The **Inversion of Control (IoC) Container** (aka `ApplicationContext`) is the heart of Spring. It:
1. Creates objects (called **beans**)
2. Manages their lifecycle
3. Injects dependencies where needed

### Stereotype Annotations — Registering Beans

| Annotation | Purpose | Layer |
|---|---|---|
| `@Component` | Generic Spring-managed bean | Any |
| `@Service` | Business logic bean | Service layer |
| `@Repository` | Data access bean (adds exception translation) | DAO layer |
| `@Controller` | MVC controller (returns views) | Web layer |
| `@RestController` | REST controller (returns JSON/XML) | Web layer |
| `@Configuration` | Declares `@Bean` factory methods | Config |

> **TIP:** `@Service`, `@Repository`, `@Controller` are all **specializations** of `@Component`. They work identically for component scanning but add semantic meaning and, in some cases, extra behavior (e.g., `@Repository` translates SQL exceptions to Spring's `DataAccessException`).

### Three Ways to Inject Dependencies

#### 1. Constructor Injection ✅ (RECOMMENDED)

```java
@Service
public class UserService {

    private final UserRepository userRepository;

    // If there's only ONE constructor, @Autowired is optional (Spring 4.3+)
    public UserService(UserRepository userRepository) {
        this.userRepository = userRepository;
    }
}
```

**Why constructor injection is best:**
- Fields can be `final` → immutable, thread-safe
- Makes dependencies explicit
- Easy to unit test (just pass mocks in constructor)
- Fails fast at startup if dependency is missing

#### 2. Setter Injection (for optional dependencies)

```java
@Service
public class NotificationService {

    private EmailService emailService;

    @Autowired
    public void setEmailService(EmailService emailService) {
        this.emailService = emailService;
    }
}
```

#### 3. Field Injection ❌ (AVOID)

```java
@Service
public class BadService {
    @Autowired  // Works but hard to test, hides dependencies
    private UserRepository userRepository;
}
```

### `@Bean` vs `@Component`

```java
@Configuration
public class AppConfig {

    @Bean  // Use when you need to configure a 3rd-party class as a bean
    public RestTemplate restTemplate() {
        return new RestTemplate();
    }

    @Bean
    public ObjectMapper objectMapper() {
        ObjectMapper mapper = new ObjectMapper();
        mapper.registerModule(new JavaTimeModule());
        mapper.disable(SerializationFeature.WRITE_DATES_AS_TIMESTAMPS);
        return mapper;
    }
}
```

**Rule of thumb:**
- Your own classes → `@Component` / `@Service` / `@Repository`
- Third-party classes → `@Bean` in a `@Configuration` class

### Qualifier & Primary

When multiple beans of the same type exist:

```java
@Service
@Primary  // This one wins by default
public class EmailNotifier implements Notifier { }

@Service
public class SmsNotifier implements Notifier { }

// Or be explicit:
@Service
public class AlertService {
    public AlertService(@Qualifier("smsNotifier") Notifier notifier) {
        // uses SmsNotifier specifically
    }
}
```

---

## 4. Configuration — `application.properties` / `application.yml`

### Properties Format

```properties
# Server
server.port=8080
server.servlet.context-path=/api

# Database
spring.datasource.url=jdbc:mysql://localhost:3306/mydb
spring.datasource.username=root
spring.datasource.password=secret
spring.datasource.driver-class-name=com.mysql.cj.jdbc.Driver

# JPA / Hibernate
spring.jpa.hibernate.ddl-auto=update
spring.jpa.show-sql=true
spring.jpa.properties.hibernate.format_sql=true
spring.jpa.properties.hibernate.dialect=org.hibernate.dialect.MySQLDialect

# Logging
logging.level.root=INFO
logging.level.com.example.demo=DEBUG
logging.level.org.hibernate.SQL=DEBUG
```

### YAML Format (equivalent)

```yaml
server:
  port: 8080
  servlet:
    context-path: /api

spring:
  datasource:
    url: jdbc:mysql://localhost:3306/mydb
    username: root
    password: secret
  jpa:
    hibernate:
      ddl-auto: update
    show-sql: true
```

### `ddl-auto` Values Explained

| Value | Behavior | When to use |
|---|---|---|
| `none` | Do nothing | Production |
| `validate` | Validate schema, don't change | Production |
| `update` | Update schema, keep data | Development |
| `create` | Drop & recreate every startup | Testing |
| `create-drop` | Create on start, drop on stop | Unit tests |

### Reading Custom Properties

```java
@Component
@ConfigurationProperties(prefix = "app")  // Binds app.* properties
public class AppProperties {

    private String name;
    private String version;
    private final Security security = new Security();

    // Getters and setters required

    public static class Security {
        private String jwtSecret;
        private long jwtExpirationMs;
        // Getters and setters
    }
}

// In application.yml:
// app:
//   name: MyApp
//   version: 1.0
//   security:
//     jwt-secret: mySecret
//     jwt-expiration-ms: 86400000
```

```java
// Or for single values:
@Value("${app.name:DefaultAppName}")  // :DefaultAppName is the fallback
private String appName;
```

### Profiles

```properties
# application-dev.properties  → activated with spring.profiles.active=dev
# application-prod.properties → activated with spring.profiles.active=prod
```

```yaml
# application.yml — multi-profile in one file
spring:
  profiles:
    active: dev

---
spring:
  config:
    activate:
      on-profile: dev
  datasource:
    url: jdbc:h2:mem:devdb

---
spring:
  config:
    activate:
      on-profile: prod
  datasource:
    url: jdbc:mysql://prod-server:3306/proddb
```

Run with profile: `java -jar app.jar --spring.profiles.active=prod`

---

## 5. Bean Scopes

| Scope | Description |
|---|---|
| `singleton` (default) | One instance per Spring container |
| `prototype` | New instance every time it's requested |
| `request` | One instance per HTTP request |
| `session` | One instance per HTTP session |
| `application` | One instance per ServletContext |

```java
@Service
@Scope("prototype")  // New instance each time
public class ReportGenerator { }
```

> **⚠️ GOTCHA:** Injecting a `prototype` bean into a `singleton` bean means the prototype is created **once** (when the singleton is created) and reused forever. Use `ObjectProvider<T>` or `@Lookup` to get fresh instances.

---

## 6. Bean Lifecycle Hooks

```java
@Component
public class CacheManager {

    @PostConstruct  // Called AFTER dependency injection is complete
    public void init() {
        // Load cache, warm up resources
    }

    @PreDestroy  // Called BEFORE the bean is destroyed
    public void cleanup() {
        // Release resources, flush cache
    }
}
```

Or implement interfaces:
```java
@Component
public class AppStartup implements CommandLineRunner {

    @Override
    public void run(String... args) {
        // Runs AFTER the application context is fully loaded
        System.out.println("Application started!");
    }
}
```

| Interface | When it runs |
|---|---|
| `CommandLineRunner` | After context loads, receives raw args |
| `ApplicationRunner` | After context loads, receives parsed `ApplicationArguments` |
| `InitializingBean` | After properties set (prefer `@PostConstruct`) |
| `DisposableBean` | On shutdown (prefer `@PreDestroy`) |

---

## Quick Reference: Most Important Annotations So Far

| Annotation | What it does |
|---|---|
| `@SpringBootApplication` | Entry point, enables auto-config + scanning |
| `@Component` | Registers class as a Spring bean |
| `@Service` | Semantic `@Component` for business logic |
| `@Repository` | Semantic `@Component` for data access |
| `@Configuration` | Class that defines `@Bean` methods |
| `@Bean` | Method-level: registers return value as bean |
| `@Autowired` | Injects a dependency (prefer constructor) |
| `@Qualifier` | Specifies which bean when multiple exist |
| `@Primary` | Default bean when multiple candidates |
| `@Value` | Injects a single config property |
| `@ConfigurationProperties` | Binds a group of properties to a POJO |
| `@Profile` | Activates bean only for specific profile |
| `@PostConstruct` | Runs after DI is complete |
| `@PreDestroy` | Runs before bean destruction |
