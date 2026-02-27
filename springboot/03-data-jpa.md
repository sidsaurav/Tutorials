# Spring Boot Tutorial — Part 3: Data Access with Spring Data JPA

## 1. Entities — Mapping Java to Database

```java
@Entity                           // Marks this class as a JPA entity (database table)
@Table(name = "users")            // Custom table name (default = class name)
public class User {

    @Id                           // Primary key
    @GeneratedValue(strategy = GenerationType.IDENTITY)  // Auto-increment
    private Long id;

    @Column(nullable = false, length = 50)  // Column constraints
    private String name;

    @Column(unique = true, nullable = false)
    private String email;

    @Column(name = "created_at", updatable = false)  // Custom column name
    private LocalDateTime createdAt;

    @Column(name = "updated_at")
    private LocalDateTime updatedAt;

    @Enumerated(EnumType.STRING)  // Store enum as string (not ordinal!)
    private Role role;

    @Lob  // Large object (TEXT/BLOB in DB)
    private String bio;

    @Transient  // NOT persisted to DB
    private String temporaryToken;

    // ALWAYS provide a no-arg constructor (JPA requirement)
    public User() {}

    // Automatic timestamps
    @PrePersist
    protected void onCreate() {
        this.createdAt = LocalDateTime.now();
        this.updatedAt = LocalDateTime.now();
    }

    @PreUpdate
    protected void onUpdate() {
        this.updatedAt = LocalDateTime.now();
    }

    // Getters & Setters...
}
```

### ID Generation Strategies

| Strategy | Description | Best for |
|---|---|---|
| `IDENTITY` | DB auto-increment | MySQL, PostgreSQL (simple) |
| `SEQUENCE` | DB sequence object | PostgreSQL, Oracle (performance) |
| `TABLE` | Separate table for IDs | Portable (slowest) |
| `UUID` | Random UUID | Distributed systems |
| `AUTO` | Let Hibernate decide | General use |

```java
// UUID example:
@Id
@GeneratedValue(strategy = GenerationType.UUID)
private UUID id;
```

---

## 2. Relationships

### One-to-Many / Many-to-One

```java
@Entity
public class User {
    @Id @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    // One user has many posts
    @OneToMany(mappedBy = "author",     // "author" field in Post entity
               cascade = CascadeType.ALL,
               orphanRemoval = true,
               fetch = FetchType.LAZY)   // LAZY = don't load until accessed
    private List<Post> posts = new ArrayList<>();

    // Helper methods (CRITICAL for bidirectional relationships!)
    public void addPost(Post post) {
        posts.add(post);
        post.setAuthor(this);
    }

    public void removePost(Post post) {
        posts.remove(post);
        post.setAuthor(null);
    }
}

@Entity
public class Post {
    @Id @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    private String title;
    private String content;

    @ManyToOne(fetch = FetchType.LAZY)  // LAZY is NOT default for *ToOne!
    @JoinColumn(name = "author_id", nullable = false)  // FK column
    private User author;
}
```

### Many-to-Many

```java
@Entity
public class Student {
    @Id @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @ManyToMany
    @JoinTable(
        name = "student_course",                             // Join table name
        joinColumns = @JoinColumn(name = "student_id"),       // FK to this entity
        inverseJoinColumns = @JoinColumn(name = "course_id")  // FK to other entity
    )
    private Set<Course> courses = new HashSet<>();
}

@Entity
public class Course {
    @Id @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @ManyToMany(mappedBy = "courses")  // Non-owning side
    private Set<Student> students = new HashSet<>();
}
```

### One-to-One

```java
@Entity
public class User {
    @OneToOne(mappedBy = "user", cascade = CascadeType.ALL)
    private UserProfile profile;
}

@Entity
public class UserProfile {
    @Id @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @OneToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "user_id", unique = true)
    private User user;
}
```

### Fetch Types & the N+1 Problem

| Type | Default for | Behavior |
|---|---|---|
| `LAZY` | `@OneToMany`, `@ManyToMany` | Load only when accessed |
| `EAGER` | `@ManyToOne`, `@OneToOne` | Always load immediately |

> **⚠️ THE N+1 PROBLEM:** If you load 100 users and each has posts with LAZY loading, accessing `user.getPosts()` for each user fires a separate query — that's 1 (users) + 100 (posts) = 101 queries! Fix with `JOIN FETCH` in JPQL or `@EntityGraph`.

```java
// Fix N+1 with JOIN FETCH:
@Query("SELECT u FROM User u JOIN FETCH u.posts WHERE u.id = :id")
Optional<User> findByIdWithPosts(@Param("id") Long id);

// Or with @EntityGraph:
@EntityGraph(attributePaths = {"posts"})
Optional<User> findById(Long id);
```

---

## 3. Spring Data JPA Repositories

```java
public interface UserRepository extends JpaRepository<User, Long> {
    //  JpaRepository gives you ALL of these for free:
    //  save(entity)              → INSERT or UPDATE
    //  saveAll(entities)         → Batch save
    //  findById(id)              → SELECT by PK (returns Optional)
    //  existsById(id)            → Check existence
    //  findAll()                 → SELECT all
    //  findAll(Pageable)         → Paginated SELECT
    //  findAll(Sort)             → Sorted SELECT
    //  count()                   → COUNT(*)
    //  deleteById(id)            → DELETE by PK
    //  delete(entity)            → DELETE
    //  deleteAll()               → DELETE all
}
```

### Repository Hierarchy

```
Repository (marker interface)
  └── CrudRepository (basic CRUD)
       └── ListCrudRepository (returns List instead of Iterable)
            └── PagingAndSortingRepository (+ pagination/sorting)
                 └── JpaRepository (+ flush, batch, deleteInBatch)
```

### Derived Query Methods (Magic Naming!)

Spring generates SQL from method names automatically:

```java
public interface UserRepository extends JpaRepository<User, Long> {

    // SELECT * FROM users WHERE email = ?
    Optional<User> findByEmail(String email);

    // SELECT * FROM users WHERE name LIKE '%value%'
    List<User> findByNameContainingIgnoreCase(String name);

    // SELECT * FROM users WHERE age BETWEEN ? AND ?
    List<User> findByAgeBetween(int min, int max);

    // SELECT * FROM users WHERE role = ? ORDER BY name ASC
    List<User> findByRoleOrderByNameAsc(Role role);

    // SELECT * FROM users WHERE active = true AND role = ?
    List<User> findByActiveTrueAndRole(Role role);

    // SELECT * FROM users WHERE name IN (?)
    List<User> findByNameIn(Collection<String> names);

    // SELECT COUNT(*) FROM users WHERE role = ?
    long countByRole(Role role);

    // Check existence
    boolean existsByEmail(String email);

    // DELETE FROM users WHERE name = ?
    void deleteByName(String name);

    // With pagination
    Page<User> findByRole(Role role, Pageable pageable);
}
```

### Query Method Keywords

| Keyword | SQL | Example |
|---|---|---|
| `findBy` | `WHERE` | `findByName(String)` |
| `And` | `AND` | `findByNameAndEmail(...)` |
| `Or` | `OR` | `findByNameOrEmail(...)` |
| `Is`, `Equals` | `=` | `findByNameIs(String)` |
| `Between` | `BETWEEN` | `findByAgeBetween(int, int)` |
| `LessThan` | `<` | `findByAgeLessThan(int)` |
| `GreaterThanEqual` | `>=` | `findByAgeGreaterThanEqual(int)` |
| `Like` | `LIKE` | `findByNameLike(String)` |
| `Containing` | `LIKE %val%` | `findByNameContaining(String)` |
| `StartingWith` | `LIKE val%` | `findByNameStartingWith(String)` |
| `In` | `IN` | `findByRoleIn(Collection)` |
| `OrderBy` | `ORDER BY` | `findByRoleOrderByNameAsc(...)` |
| `Not` | `<>` | `findByRoleNot(Role)` |
| `IsNull` | `IS NULL` | `findByEmailIsNull()` |
| `True`/`False` | `= true/false` | `findByActiveTrue()` |
| `Top`/`First` | `LIMIT` | `findTop5ByOrderByCreatedAtDesc()` |

### Custom JPQL Queries

```java
@Query("SELECT u FROM User u WHERE u.email = :email AND u.active = true")
Optional<User> findActiveByEmail(@Param("email") String email);

@Query("SELECT u FROM User u WHERE u.name LIKE %:keyword% OR u.email LIKE %:keyword%")
Page<User> search(@Param("keyword") String keyword, Pageable pageable);

// Use nativeQuery for raw SQL
@Query(value = "SELECT * FROM users WHERE YEAR(created_at) = :year", nativeQuery = true)
List<User> findByCreatedYear(@Param("year") int year);

// UPDATE/DELETE queries need @Modifying
@Modifying
@Transactional
@Query("UPDATE User u SET u.active = false WHERE u.lastLoginAt < :date")
int deactivateInactiveUsers(@Param("date") LocalDateTime date);
```

---

## 4. Pagination & Sorting

```java
// In Controller:
@GetMapping
public Page<UserDTO> getUsers(
    @RequestParam(defaultValue = "0") int page,
    @RequestParam(defaultValue = "10") int size,
    @RequestParam(defaultValue = "createdAt") String sortBy,
    @RequestParam(defaultValue = "desc") String direction
) {
    Sort sort = direction.equalsIgnoreCase("asc")
        ? Sort.by(sortBy).ascending()
        : Sort.by(sortBy).descending();

    Pageable pageable = PageRequest.of(page, size, sort);
    return userService.findAll(pageable);
}

// In Repository:
Page<User> findByRole(Role role, Pageable pageable);
```

### Page Response Structure (automatic JSON)

```json
{
    "content": [ /* list of items */ ],
    "pageable": { "pageNumber": 0, "pageSize": 10 },
    "totalElements": 100,
    "totalPages": 10,
    "first": true,
    "last": false,
    "number": 0,
    "size": 10,
    "numberOfElements": 10
}
```

---

## 5. Auditing (Auto-track created/updated)

```java
// 1. Enable auditing
@Configuration
@EnableJpaAuditing
public class JpaConfig {
    @Bean
    public AuditorAware<String> auditorAware() {
        return () -> Optional.ofNullable(
            SecurityContextHolder.getContext().getAuthentication().getName()
        );
    }
}

// 2. Create a base entity
@MappedSuperclass
@EntityListeners(AuditingEntityListener.class)
public abstract class BaseEntity {

    @CreatedDate
    @Column(updatable = false)
    private LocalDateTime createdAt;

    @LastModifiedDate
    private LocalDateTime updatedAt;

    @CreatedBy
    @Column(updatable = false)
    private String createdBy;

    @LastModifiedBy
    private String updatedBy;
}

// 3. Extend it
@Entity
public class User extends BaseEntity {
    // createdAt, updatedAt, createdBy, updatedBy are auto-managed!
}
```

---

## 6. Using H2 for Quick Development

Add `com.h2database:h2` dependency (scope: runtime).

```properties
# application-dev.properties
spring.datasource.url=jdbc:h2:mem:devdb
spring.datasource.driver-class-name=org.h2.Driver
spring.datasource.username=sa
spring.datasource.password=
spring.h2.console.enabled=true          # Access at /h2-console
spring.h2.console.path=/h2-console
spring.jpa.database-platform=org.hibernate.dialect.H2Dialect
spring.jpa.hibernate.ddl-auto=create-drop
```

Access the H2 console at `http://localhost:8080/h2-console` with JDBC URL `jdbc:h2:mem:devdb`.
