# Spring Boot Tutorial — Part 6: Testing

## 1. Test Setup

Spring Boot Starter Test includes: **JUnit 5**, **Mockito**, **Spring Test**, **AssertJ**, **Hamcrest**, **JSONPath**.

```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-test</artifactId>
    <scope>test</scope>
</dependency>
```

---

## 2. Unit Tests (No Spring Context)

```java
// Test pure business logic — fast, no Spring needed
class UserServiceTest {

    private UserService userService;

    @Mock
    private UserRepository userRepository;

    @BeforeEach
    void setUp() {
        MockitoAnnotations.openMocks(this);
        userService = new UserServiceImpl(userRepository);
    }

    @Test
    void findById_WhenUserExists_ReturnsUser() {
        // Arrange
        User user = new User(1L, "John", "john@email.com");
        when(userRepository.findById(1L)).thenReturn(Optional.of(user));

        // Act
        UserDTO result = userService.findById(1L);

        // Assert
        assertThat(result).isNotNull();
        assertThat(result.getName()).isEqualTo("John");
        verify(userRepository, times(1)).findById(1L);
    }

    @Test
    void findById_WhenNotExists_ThrowsException() {
        when(userRepository.findById(99L)).thenReturn(Optional.empty());

        assertThrows(ResourceNotFoundException.class,
            () -> userService.findById(99L));
    }
}
```

---

## 3. Integration Tests (`@SpringBootTest`)

```java
@SpringBootTest                           // Loads full application context
@AutoConfigureMockMvc                     // Configures MockMvc for HTTP testing
@Transactional                            // Rolls back after each test
class UserControllerIntegrationTest {

    @Autowired
    private MockMvc mockMvc;

    @Autowired
    private ObjectMapper objectMapper;

    @Autowired
    private UserRepository userRepository;

    @Test
    void createUser_ReturnsCreated() throws Exception {
        CreateUserRequest req = new CreateUserRequest("John", "john@email.com", "pass1234");

        mockMvc.perform(post("/api/users")
                .contentType(MediaType.APPLICATION_JSON)
                .content(objectMapper.writeValueAsString(req)))
            .andExpect(status().isCreated())
            .andExpect(jsonPath("$.name").value("John"))
            .andExpect(jsonPath("$.email").value("john@email.com"));
    }

    @Test
    void getUser_WhenNotExists_Returns404() throws Exception {
        mockMvc.perform(get("/api/users/999"))
            .andExpect(status().isNotFound())
            .andExpect(jsonPath("$.message").exists());
    }
}
```

---

## 4. Slice Tests (Load Only What You Need)

| Annotation | What it loads | Use for |
|---|---|---|
| `@WebMvcTest` | Controllers + filters only | Testing controllers |
| `@DataJpaTest` | JPA + embedded DB | Testing repositories |
| `@WebFluxTest` | WebFlux controllers | Reactive controllers |
| `@JsonTest` | Jackson ObjectMapper | JSON serialization |

```java
// Controller slice test (only loads UserController, not the whole app)
@WebMvcTest(UserController.class)
class UserControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockBean  // Creates a mock and puts it in the Spring context
    private UserService userService;

    @Test
    void getAllUsers_ReturnsList() throws Exception {
        when(userService.findAll()).thenReturn(
            List.of(new UserDTO(1L, "John", "john@email.com"))
        );

        mockMvc.perform(get("/api/users"))
            .andExpect(status().isOk())
            .andExpect(jsonPath("$", hasSize(1)))
            .andExpect(jsonPath("$[0].name").value("John"));
    }
}

// Repository slice test
@DataJpaTest  // Uses embedded H2 by default
class UserRepositoryTest {

    @Autowired
    private UserRepository userRepository;

    @Autowired
    private TestEntityManager entityManager;

    @Test
    void findByEmail_ReturnsUser() {
        User user = new User();
        user.setName("John");
        user.setEmail("john@email.com");
        entityManager.persistAndFlush(user);

        Optional<User> found = userRepository.findByEmail("john@email.com");

        assertThat(found).isPresent();
        assertThat(found.get().getName()).isEqualTo("John");
    }
}
```

---

## 5. Key Testing Annotations

| Annotation | Purpose |
|---|---|
| `@SpringBootTest` | Full integration test with entire context |
| `@WebMvcTest` | Slim test: controllers only |
| `@DataJpaTest` | Slim test: JPA repositories only |
| `@MockBean` | Replace a bean in Spring context with a mock |
| `@SpyBean` | Wrap a real bean with spy capabilities |
| `@Transactional` | Rollback after each test |
| `@Sql` | Execute SQL scripts before/after test |
| `@TestPropertySource` | Override properties for tests |
| `@ActiveProfiles("test")` | Activate test profile |
