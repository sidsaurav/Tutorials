# Spring Boot Tutorial — Part 2: Building REST APIs

## 1. Controllers — `@RestController` vs `@Controller`

```java
@Controller       // Returns VIEW names (HTML templates)
@RestController   // Returns DATA directly (JSON/XML) — equivalent to @Controller + @ResponseBody
```

### Your First REST Controller

```java
@RestController
@RequestMapping("/api/users")  // Base path for all endpoints in this controller
public class UserController {

    private final UserService userService;

    public UserController(UserService userService) {
        this.userService = userService;
    }

    // GET /api/users
    @GetMapping
    public List<UserDTO> getAllUsers() {
        return userService.findAll();
    }

    // GET /api/users/5
    @GetMapping("/{id}")
    public UserDTO getUser(@PathVariable Long id) {
        return userService.findById(id);
    }

    // POST /api/users
    @PostMapping
    @ResponseStatus(HttpStatus.CREATED)  // Returns 201 instead of default 200
    public UserDTO createUser(@Valid @RequestBody CreateUserRequest request) {
        return userService.create(request);
    }

    // PUT /api/users/5
    @PutMapping("/{id}")
    public UserDTO updateUser(@PathVariable Long id,
                              @Valid @RequestBody UpdateUserRequest request) {
        return userService.update(id, request);
    }

    // DELETE /api/users/5
    @DeleteMapping("/{id}")
    @ResponseStatus(HttpStatus.NO_CONTENT)  // 204
    public void deleteUser(@PathVariable Long id) {
        userService.delete(id);
    }
}
```

### HTTP Method Annotations

| Annotation | HTTP Method | Typical Use |
|---|---|---|
| `@GetMapping` | GET | Read / fetch data |
| `@PostMapping` | POST | Create new resource |
| `@PutMapping` | PUT | Full update of resource |
| `@PatchMapping` | PATCH | Partial update |
| `@DeleteMapping` | DELETE | Remove resource |

---

## 2. Request Parameters — Every Way to Get Data

### Path Variables

```java
@GetMapping("/users/{userId}/posts/{postId}")
public Post getPost(@PathVariable Long userId,
                    @PathVariable("postId") Long pid) {  // rename with value
    // /users/5/posts/10 → userId=5, pid=10
}
```

### Query Parameters

```java
@GetMapping("/users")
public List<User> search(
    @RequestParam String name,                           // required
    @RequestParam(required = false) String email,        // optional (null if absent)
    @RequestParam(defaultValue = "0") int page,          // with default
    @RequestParam(defaultValue = "10") int size
) {
    // /users?name=John&page=1&size=20
}
```

### Request Body (JSON)

```java
@PostMapping("/users")
public User create(@RequestBody CreateUserRequest req) {
    // Spring automatically deserializes JSON → Java object (via Jackson)
}
```

### Request Headers

```java
@GetMapping("/secure")
public String secure(@RequestHeader("Authorization") String authHeader,
                     @RequestHeader(value = "X-Custom", required = false) String custom) {
    return "Token: " + authHeader;
}
```

### Request Attributes & Cookies

```java
@GetMapping("/dashboard")
public String dashboard(@CookieValue("sessionId") String sessionId,
                        @RequestAttribute("userId") Long userId) { }
```

---

## 3. Response Handling with `ResponseEntity`

`ResponseEntity` gives you full control over HTTP status, headers, and body:

```java
@GetMapping("/{id}")
public ResponseEntity<UserDTO> getUser(@PathVariable Long id) {
    return userService.findById(id)
        .map(user -> ResponseEntity.ok(user))                    // 200
        .orElse(ResponseEntity.notFound().build());              // 404
}

@PostMapping
public ResponseEntity<UserDTO> create(@Valid @RequestBody CreateUserRequest req) {
    UserDTO created = userService.create(req);
    URI location = URI.create("/api/users/" + created.getId());
    return ResponseEntity
        .created(location)    // 201 + Location header
        .body(created);
}

@DeleteMapping("/{id}")
public ResponseEntity<Void> delete(@PathVariable Long id) {
    userService.delete(id);
    return ResponseEntity.noContent().build();  // 204
}
```

> **TIP:** Use `ResponseEntity` when you need to vary the HTTP status or set custom headers. For simple cases, returning the object directly with `@ResponseStatus` is cleaner.

---

## 4. Validation with Bean Validation (JSR 380)

Add `spring-boot-starter-validation` dependency.

### Validation Annotations

```java
public class CreateUserRequest {

    @NotBlank(message = "Name is required")
    @Size(min = 2, max = 50, message = "Name must be 2-50 characters")
    private String name;

    @NotBlank(message = "Email is required")
    @Email(message = "Must be a valid email")
    private String email;

    @NotNull
    @Min(value = 18, message = "Must be at least 18")
    @Max(value = 120)
    private Integer age;

    @Pattern(regexp = "^\\+?[1-9]\\d{1,14}$", message = "Invalid phone number")
    private String phone;

    @NotEmpty(message = "Roles cannot be empty")
    private List<@NotBlank String> roles;  // Validate elements too!

    // Getters & Setters
}
```

### Common Validation Annotations

| Annotation | Applies to | Checks |
|---|---|---|
| `@NotNull` | Any | Not null |
| `@NotBlank` | String | Not null, not empty, not whitespace |
| `@NotEmpty` | String/Collection | Not null and not empty |
| `@Size(min, max)` | String/Collection | Length/size within range |
| `@Min` / `@Max` | Number | Minimum/maximum value |
| `@Email` | String | Valid email format |
| `@Pattern(regexp)` | String | Matches regex |
| `@Past` / `@Future` | Date/Time | Date in past/future |
| `@Positive` / `@PositiveOrZero` | Number | > 0 or >= 0 |
| `@Valid` | Object | Recurse into nested object |

### Triggering Validation in Controller

```java
@PostMapping
public UserDTO create(@Valid @RequestBody CreateUserRequest request) {
    // @Valid triggers validation. If it fails → MethodArgumentNotValidException (400)
    return userService.create(request);
}
```

### Custom Validator

```java
// 1. Define the annotation
@Target({FIELD})
@Retention(RUNTIME)
@Constraint(validatedBy = UniqueEmailValidator.class)
public @interface UniqueEmail {
    String message() default "Email already exists";
    Class<?>[] groups() default {};
    Class<? extends Payload>[] payload() default {};
}

// 2. Implement the validator
@Component
public class UniqueEmailValidator implements ConstraintValidator<UniqueEmail, String> {

    private final UserRepository userRepo;

    public UniqueEmailValidator(UserRepository userRepo) {
        this.userRepo = userRepo;
    }

    @Override
    public boolean isValid(String email, ConstraintValidatorContext ctx) {
        return email != null && !userRepo.existsByEmail(email);
    }
}

// 3. Use it
public class CreateUserRequest {
    @UniqueEmail
    private String email;
}
```

---

## 5. Global Exception Handling with `@ControllerAdvice`

This is the **single best pattern** in Spring Boot for clean error responses:

```java
@RestControllerAdvice  // = @ControllerAdvice + @ResponseBody
public class GlobalExceptionHandler {

    // Handle validation errors
    @ExceptionHandler(MethodArgumentNotValidException.class)
    @ResponseStatus(HttpStatus.BAD_REQUEST)
    public Map<String, Object> handleValidation(MethodArgumentNotValidException ex) {
        Map<String, String> fieldErrors = new HashMap<>();
        ex.getBindingResult().getFieldErrors().forEach(error ->
            fieldErrors.put(error.getField(), error.getDefaultMessage())
        );
        return Map.of(
            "status", 400,
            "error", "Validation Failed",
            "fieldErrors", fieldErrors
        );
    }

    // Handle custom "not found" exception
    @ExceptionHandler(ResourceNotFoundException.class)
    @ResponseStatus(HttpStatus.NOT_FOUND)
    public Map<String, Object> handleNotFound(ResourceNotFoundException ex) {
        return Map.of(
            "status", 404,
            "error", "Not Found",
            "message", ex.getMessage()
        );
    }

    // Catch-all for unhandled exceptions
    @ExceptionHandler(Exception.class)
    @ResponseStatus(HttpStatus.INTERNAL_SERVER_ERROR)
    public Map<String, Object> handleGeneral(Exception ex) {
        return Map.of(
            "status", 500,
            "error", "Internal Server Error",
            "message", "Something went wrong"
        );
        // Log the actual error: log.error("Unexpected error", ex);
    }
}
```

### Custom Exception

```java
@ResponseStatus(HttpStatus.NOT_FOUND)  // Optional: sets status when thrown
public class ResourceNotFoundException extends RuntimeException {
    public ResourceNotFoundException(String resource, Long id) {
        super(resource + " not found with id: " + id);
    }
}

// Usage in service:
public UserDTO findById(Long id) {
    return userRepository.findById(id)
        .map(this::toDTO)
        .orElseThrow(() -> new ResourceNotFoundException("User", id));
}
```

---

## 6. Jackson JSON Customization

Spring Boot uses **Jackson** for JSON serialization/deserialization.

### Key Annotations

```java
public class UserDTO {

    private Long id;

    @JsonProperty("full_name")  // JSON key will be "full_name" instead of "name"
    private String name;

    @JsonIgnore  // Never include in JSON output
    private String password;

    @JsonInclude(JsonInclude.Include.NON_NULL)  // Omit if null
    private String middleName;

    @JsonFormat(pattern = "yyyy-MM-dd HH:mm:ss")
    private LocalDateTime createdAt;
}
```

### Global Jackson Config

```properties
# application.properties
spring.jackson.serialization.write-dates-as-timestamps=false
spring.jackson.default-property-inclusion=non_null
spring.jackson.deserialization.fail-on-unknown-properties=false
spring.jackson.property-naming-strategy=SNAKE_CASE
```

---

## 7. The Service Layer Pattern

```java
public interface UserService {
    List<UserDTO> findAll();
    UserDTO findById(Long id);
    UserDTO create(CreateUserRequest request);
    UserDTO update(Long id, UpdateUserRequest request);
    void delete(Long id);
}

@Service
@Transactional(readOnly = true)  // Default: read-only transactions
public class UserServiceImpl implements UserService {

    private final UserRepository userRepository;

    public UserServiceImpl(UserRepository userRepository) {
        this.userRepository = userRepository;
    }

    @Override
    public List<UserDTO> findAll() {
        return userRepository.findAll()
            .stream()
            .map(this::toDTO)
            .toList();
    }

    @Override
    public UserDTO findById(Long id) {
        return userRepository.findById(id)
            .map(this::toDTO)
            .orElseThrow(() -> new ResourceNotFoundException("User", id));
    }

    @Override
    @Transactional  // Writable transaction (overrides class-level readOnly)
    public UserDTO create(CreateUserRequest req) {
        User user = new User();
        user.setName(req.getName());
        user.setEmail(req.getEmail());
        User saved = userRepository.save(user);
        return toDTO(saved);
    }

    @Override
    @Transactional
    public void delete(Long id) {
        if (!userRepository.existsById(id)) {
            throw new ResourceNotFoundException("User", id);
        }
        userRepository.deleteById(id);
    }

    private UserDTO toDTO(User user) {
        return new UserDTO(user.getId(), user.getName(), user.getEmail());
    }
}
```

### `@Transactional` Key Points

| Property | Default | Description |
|---|---|---|
| `readOnly` | `false` | Optimization hint for DB |
| `propagation` | `REQUIRED` | Join existing or create new txn |
| `isolation` | `DEFAULT` | DB isolation level |
| `rollbackFor` | `RuntimeException` | Which exceptions trigger rollback |
| `timeout` | `-1` (none) | Transaction timeout in seconds |

> **⚠️ GOTCHA:** `@Transactional` only works on **public** methods called from **outside the class**. Calling a `@Transactional` method from within the same class **bypasses the proxy** and the transaction won't apply!

```java
// THIS WON'T WORK:
@Service
public class OrderService {
    public void process() {
        this.saveOrder();  // ← No transaction! Internal call bypasses proxy
    }

    @Transactional
    public void saveOrder() { }
}
```

---

## 8. DTOs — Why and How

Never expose your entity classes directly in API responses.

**Why DTOs?**
- Security: Don't expose internal fields (password, internal IDs)
- Flexibility: API shape can differ from DB schema
- Stability: Change entities without breaking the API
- Validation: Different validation rules for create vs update

```java
// Response DTO
public record UserDTO(
    Long id,
    String name,
    String email,
    LocalDateTime createdAt
) {}

// Request DTO
public record CreateUserRequest(
    @NotBlank String name,
    @Email @NotBlank String email,
    @NotBlank @Size(min = 8) String password
) {}

// Update DTO (all fields optional)
public record UpdateUserRequest(
    @Size(min = 2, max = 50) String name,
    @Email String email
) {}
```

> **TIP:** Java `record` classes are perfect for DTOs — they're immutable with auto-generated `equals()`, `hashCode()`, `toString()`, and getters.
