# Spring Boot Tutorial — Part 7: Advanced Topics, Tips & Tricks

## 1. Filters vs Interceptors vs AOP

| Mechanism | Level | Use case |
|---|---|---|
| **Servlet Filter** | Servlet (before Spring) | Logging, security, compression |
| **HandlerInterceptor** | Spring MVC | Auth checks, locale, model enrichment |
| **AOP `@Aspect`** | Method-level | Cross-cutting: logging, timing, auditing |

### Custom Filter

```java
@Component
@Order(1)  // Lower number = runs first
public class RequestTimingFilter extends OncePerRequestFilter {

    @Override
    protected void doFilterInternal(HttpServletRequest request,
                                    HttpServletResponse response,
                                    FilterChain filterChain) throws ServletException, IOException {
        long start = System.currentTimeMillis();
        filterChain.doFilter(request, response);  // Continue the chain
        long duration = System.currentTimeMillis() - start;
        log.info("{} {} took {}ms", request.getMethod(), request.getRequestURI(), duration);
    }
}
```

### AOP (Aspect-Oriented Programming)

Add `spring-boot-starter-aop`.

```java
@Aspect
@Component
public class LoggingAspect {

    // Run BEFORE any method in any @Service class
    @Before("within(@org.springframework.stereotype.Service *)")
    public void logBefore(JoinPoint joinPoint) {
        log.info("Calling: {}.{}()",
            joinPoint.getTarget().getClass().getSimpleName(),
            joinPoint.getSignature().getName());
    }

    // Run AROUND (wraps the method) — can modify behavior
    @Around("@annotation(Timed)")  // Only methods annotated with @Timed
    public Object timeMethod(ProceedingJoinPoint joinPoint) throws Throwable {
        long start = System.currentTimeMillis();
        Object result = joinPoint.proceed();  // Execute the actual method
        long duration = System.currentTimeMillis() - start;
        log.info("{} took {}ms", joinPoint.getSignature().getName(), duration);
        return result;
    }
}

// Custom annotation for AOP targeting
@Target(ElementType.METHOD)
@Retention(RetentionPolicy.RUNTIME)
public @interface Timed {}

// Usage:
@Service
public class ReportService {
    @Timed  // Will log execution time
    public Report generate() { }
}
```

### Pointcut Expressions Cheat Sheet

| Expression | Matches |
|---|---|
| `execution(* com.example.service.*.*(..))` | All methods in service package |
| `within(@Service *)` | All methods in @Service classes |
| `@annotation(Timed)` | Methods with @Timed annotation |
| `bean(userService)` | All methods on bean named userService |

---

## 2. Scheduling

```java
@Configuration
@EnableScheduling
public class SchedulingConfig { }

@Service
public class CleanupService {

    @Scheduled(fixedRate = 60000)          // Every 60 seconds
    public void cleanTempFiles() { }

    @Scheduled(fixedDelay = 30000)         // 30s after LAST execution finishes
    public void syncData() { }

    @Scheduled(cron = "0 0 2 * * ?")      // Every day at 2:00 AM
    public void generateDailyReport() { }

    @Scheduled(cron = "0 */15 * * * ?")   // Every 15 minutes
    public void checkHealth() { }
}
```

### Cron Format: `second minute hour day month weekday`

| Field | Values |
|---|---|
| Second | 0-59 |
| Minute | 0-59 |
| Hour | 0-23 |
| Day | 1-31 |
| Month | 1-12 |
| Weekday | 0-7 (0 and 7 = Sunday) |

---

## 3. Async Processing

```java
@Configuration
@EnableAsync
public class AsyncConfig {

    @Bean
    public Executor taskExecutor() {
        ThreadPoolTaskExecutor executor = new ThreadPoolTaskExecutor();
        executor.setCorePoolSize(5);
        executor.setMaxPoolSize(10);
        executor.setQueueCapacity(25);
        executor.setThreadNamePrefix("Async-");
        executor.initialize();
        return executor;
    }
}

@Service
public class EmailService {

    @Async  // Runs in a separate thread — returns immediately
    public CompletableFuture<Boolean> sendEmail(String to, String subject) {
        // ... slow email sending logic ...
        return CompletableFuture.completedFuture(true);
    }
}
```

> **⚠️ GOTCHA:** Same as `@Transactional` — `@Async` uses proxies. Calling an `@Async` method from within the same class won't work!

---

## 4. Caching

```java
@Configuration
@EnableCaching
public class CacheConfig { }

@Service
public class ProductService {

    @Cacheable(value = "products", key = "#id")  // Cache result
    public Product findById(Long id) {
        log.info("DB query for product {}", id);  // Only logged on FIRST call
        return productRepo.findById(id).orElseThrow();
    }

    @CachePut(value = "products", key = "#product.id")  // Update cache
    public Product update(Product product) {
        return productRepo.save(product);
    }

    @CacheEvict(value = "products", key = "#id")  // Remove from cache
    public void delete(Long id) {
        productRepo.deleteById(id);
    }

    @CacheEvict(value = "products", allEntries = true)  // Clear entire cache
    public void clearCache() { }
}
```

---

## 5. File Upload & Download

```java
@RestController
@RequestMapping("/api/files")
public class FileController {

    @Value("${app.upload-dir:./uploads}")
    private String uploadDir;

    @PostMapping("/upload")
    public ResponseEntity<String> upload(@RequestParam("file") MultipartFile file) throws IOException {
        if (file.isEmpty()) {
            return ResponseEntity.badRequest().body("File is empty");
        }
        String filename = UUID.randomUUID() + "_" + file.getOriginalFilename();
        Path path = Paths.get(uploadDir).resolve(filename);
        Files.createDirectories(path.getParent());
        Files.copy(file.getInputStream(), path);
        return ResponseEntity.ok("Uploaded: " + filename);
    }

    @GetMapping("/download/{filename}")
    public ResponseEntity<Resource> download(@PathVariable String filename) throws IOException {
        Path path = Paths.get(uploadDir).resolve(filename);
        Resource resource = new UrlResource(path.toUri());
        return ResponseEntity.ok()
            .header(HttpHeaders.CONTENT_DISPOSITION,
                    "attachment; filename=\"" + filename + "\"")
            .body(resource);
    }
}
```

```properties
# Max file size settings
spring.servlet.multipart.max-file-size=10MB
spring.servlet.multipart.max-request-size=20MB
```

---

## 6. Calling External APIs with `RestClient` (Spring 6.1+)

```java
@Configuration
public class RestClientConfig {
    @Bean
    public RestClient restClient() {
        return RestClient.builder()
            .baseUrl("https://api.example.com")
            .defaultHeader("Accept", "application/json")
            .build();
    }
}

@Service
public class ExternalApiService {

    private final RestClient restClient;

    public ExternalApiService(RestClient restClient) {
        this.restClient = restClient;
    }

    public UserDTO getUser(Long id) {
        return restClient.get()
            .uri("/users/{id}", id)
            .retrieve()
            .body(UserDTO.class);
    }

    public UserDTO createUser(CreateUserRequest request) {
        return restClient.post()
            .uri("/users")
            .contentType(MediaType.APPLICATION_JSON)
            .body(request)
            .retrieve()
            .body(UserDTO.class);
    }
}
```

---

## 7. Actuator — Production Monitoring

Add `spring-boot-starter-actuator`.

```properties
# Expose all endpoints
management.endpoints.web.exposure.include=health,info,metrics,env,loggers

# Health details
management.endpoint.health.show-details=always

# Custom info
info.app.name=@project.name@
info.app.version=@project.version@
```

**Key Endpoints:**

| Endpoint | Purpose |
|---|---|
| `/actuator/health` | App health status |
| `/actuator/info` | App info |
| `/actuator/metrics` | Metrics (memory, CPU, etc.) |
| `/actuator/env` | Environment properties |
| `/actuator/loggers` | View/change log levels at runtime |
| `/actuator/beans` | All registered beans |

---

## 8. DevTools — Faster Development

Add `spring-boot-devtools` (scope: `runtime`, `optional: true`).

**Features:**
- **Auto restart** when code changes
- **LiveReload** for browser auto-refresh
- `spring.thymeleaf.cache=false` by default
- H2 console auto-enabled

---

## 9. Essential Tips & Tricks

### Lombok — Kill Boilerplate

```java
@Data           // Generates getters, setters, toString, equals, hashCode
@NoArgsConstructor
@AllArgsConstructor
@Builder        // Enables builder pattern: User.builder().name("John").build()
@Entity
public class User {
    @Id @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;
    private String name;
    private String email;
}
```

> **WARNING:** Don't use `@Data` on JPA entities in production — `equals`/`hashCode` on lazy-loaded fields cause issues. Use `@Getter`, `@Setter`, `@ToString(exclude = "posts")` separately.

### Common Mistakes to Avoid

| Mistake | Fix |
|---|---|
| Circular dependencies | Redesign, use `@Lazy`, or break the cycle |
| Exposing entities in APIs | Always use DTOs |
| Not using `Optional` correctly | Never call `.get()` without `.isPresent()` — use `.orElseThrow()` |
| Catching generic `Exception` | Catch specific exceptions |
| `@Transactional` on private methods | Only works on public methods |
| Internal `@Transactional` calls | Call from another bean, not `this` |
| N+1 queries | Use `JOIN FETCH` or `@EntityGraph` |
| `EAGER` fetch everywhere | Default to `LAZY`, load explicitly when needed |
| Using `==` for entity comparison | Override `equals`/`hashCode` using business key |
| Not closing resources | Use try-with-resources |

### Useful Application Properties

```properties
# Pretty JSON output
spring.jackson.serialization.indent-output=true

# Show full error details (DEV only)
server.error.include-message=always
server.error.include-binding-errors=always
server.error.include-stacktrace=on_param

# Access log
server.tomcat.accesslog.enabled=true

# Graceful shutdown
server.shutdown=graceful
spring.lifecycle.timeout-per-shutdown-phase=30s

# Compression
server.compression.enabled=true
server.compression.mime-types=application/json,text/html,text/css
```

### Project Packaging & Deployment

```bash
# Build executable JAR
./mvnw clean package -DskipTests

# Run
java -jar target/myapp-0.0.1-SNAPSHOT.jar

# Run with profile
java -jar target/myapp.jar --spring.profiles.active=prod

# Run with custom port
java -jar target/myapp.jar --server.port=9090
```

---

## 10. Complete `pom.xml` for a Web App

```xml
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0
         https://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>

    <parent>
        <groupId>org.springframework.boot</groupId>
        <artifactId>spring-boot-starter-parent</artifactId>
        <version>3.3.0</version>
    </parent>

    <groupId>com.example</groupId>
    <artifactId>webapp</artifactId>
    <version>0.0.1-SNAPSHOT</version>

    <properties>
        <java.version>21</java.version>
    </properties>

    <dependencies>
        <!-- Web (Tomcat + Spring MVC) -->
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-web</artifactId>
        </dependency>

        <!-- Template engine -->
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-thymeleaf</artifactId>
        </dependency>

        <!-- JPA + Hibernate -->
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-data-jpa</artifactId>
        </dependency>

        <!-- Validation -->
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-validation</artifactId>
        </dependency>

        <!-- Security -->
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-security</artifactId>
        </dependency>

        <!-- Actuator (production monitoring) -->
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-actuator</artifactId>
        </dependency>

        <!-- MySQL driver -->
        <dependency>
            <groupId>com.mysql</groupId>
            <artifactId>mysql-connector-j</artifactId>
            <scope>runtime</scope>
        </dependency>

        <!-- H2 for dev/test -->
        <dependency>
            <groupId>com.h2database</groupId>
            <artifactId>h2</artifactId>
            <scope>runtime</scope>
        </dependency>

        <!-- DevTools -->
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-devtools</artifactId>
            <scope>runtime</scope>
            <optional>true</optional>
        </dependency>

        <!-- Lombok -->
        <dependency>
            <groupId>org.projectlombok</groupId>
            <artifactId>lombok</artifactId>
            <optional>true</optional>
        </dependency>

        <!-- Test -->
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-test</artifactId>
            <scope>test</scope>
        </dependency>
        <dependency>
            <groupId>org.springframework.security</groupId>
            <artifactId>spring-security-test</artifactId>
            <scope>test</scope>
        </dependency>
    </dependencies>

    <build>
        <plugins>
            <plugin>
                <groupId>org.springframework.boot</groupId>
                <artifactId>spring-boot-maven-plugin</artifactId>
                <configuration>
                    <excludes>
                        <exclude>
                            <groupId>org.projectlombok</groupId>
                            <artifactId>lombok</artifactId>
                        </exclude>
                    </excludes>
                </configuration>
            </plugin>
        </plugins>
    </build>
</project>
```

---

## Quick Reference: All Important Interfaces

| Interface | Purpose |
|---|---|
| `JpaRepository<T, ID>` | Full CRUD + pagination for entities |
| `UserDetailsService` | Load user data for Spring Security |
| `UserDetails` | Represents an authenticated user |
| `HandlerInterceptor` | Pre/post processing of MVC requests |
| `WebMvcConfigurer` | Customize Spring MVC config |
| `CommandLineRunner` | Run code on app startup |
| `ApplicationRunner` | Run code on startup (parsed args) |
| `Filter` / `OncePerRequestFilter` | Servlet-level request filtering |
| `ConstraintValidator<A, T>` | Custom validation logic |
| `Converter<S, T>` | Type conversion |
| `InitializingBean` | Post-initialization callback |
| `DisposableBean` | Pre-destruction callback |
| `ErrorController` | Custom error page handling |
| `ResponseBodyAdvice<T>` | Modify response body globally |
| `RequestBodyAdvice` | Modify request body globally |
| `AuditorAware<T>` | Provide current user for JPA auditing |
