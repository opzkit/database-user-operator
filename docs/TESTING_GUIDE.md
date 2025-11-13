# Testing Guide for Database Operator

This guide covers best practices for testing the Database Operator.

## Testing Strategy

The operator uses a two-layer testing approach:

1. **Unit Tests**: Fast, no external dependencies, test individual functions in isolation
2. **Integration Tests**: Real Kubernetes cluster with databases, test complete workflows

## Test Structure

```
internal/controller/
├── database_controller.go
└── database_controller_unit_test.go     # Unit tests (no external dependencies)

internal/database/
├── postgres.go
├── postgres_test.go                     # Unit tests (string parsing, password generation)
├── mysql.go
└── mysql_test.go                        # Unit tests (string parsing, DSN conversion)

internal/secrets/
├── aws_secrets_manager.go
└── aws_secrets_manager_test.go          # Unit tests (region validation, JSON serialization)

test/integration/
├── integration_test.go                  # Integration test suite setup
└── operator_test.go                     # Full E2E tests with real cluster
```

## Unit Testing Best Practices

### 1. Use Fake Clients

```go
import (
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDatabaseReconciler_ValidateDatabase(t *testing.T) {
    scheme := runtime.NewScheme()
    _ = v1alpha1.AddToScheme(scheme)

    fakeClient := fake.NewClientBuilder().
        WithScheme(scheme).
        Build()

    reconciler := &DatabaseReconciler{
        Client: fakeClient,
        Scheme: scheme,
    }

    // Test your logic
}
```

### 2. Mock External Dependencies

Create interfaces for external services:

```go
// Interface for database operations
type DatabaseClient interface {
    CreateDatabase(ctx context.Context, name, owner string) error
    CreateUser(ctx context.Context, username, password string) error
    DropDatabase(ctx context.Context, name string) error
}

// Mock implementation
type MockDatabaseClient struct {
    CreateDatabaseFunc func(ctx context.Context, name, owner string) error
    CreateUserFunc     func(ctx context.Context, username, password string) error
}

func (m *MockDatabaseClient) CreateDatabase(ctx context.Context, name, owner string) error {
    if m.CreateDatabaseFunc != nil {
        return m.CreateDatabaseFunc(ctx, name, owner)
    }
    return nil
}
```

### 3. Test Validation Logic

```go
func TestValidation(t *testing.T) {
    tests := []struct {
        name    string
        db      *v1alpha1.Database
        wantErr bool
    }{
        {
            name: "valid database",
            db: &v1alpha1.Database{
                Spec: v1alpha1.DatabaseSpec{
                    Engine:       "postgres",
                    DatabaseName: "test_db",
                    ConnectionStringSecretRef: v1alpha1.SecretKeyReference{
                        Name: "admin-secret",
                    },
                },
            },
            wantErr: false,
        },
        {
            name: "empty database name",
            db: &v1alpha1.Database{
                Spec: v1alpha1.DatabaseSpec{
                    Engine:       "postgres",
                    DatabaseName: "",
                },
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateDatabase(tt.db)
            if (err != nil) != tt.wantErr {
                t.Errorf("validateDatabase() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

## Integration Testing with Real Cluster

Integration tests use a real Kubernetes cluster (kind) with actual database instances (PostgreSQL, MySQL) and LocalStack (for AWS Secrets Manager emulation). This provides the most realistic testing environment.

### Test Organization

Tests are located in `test/integration/operator_test.go` and use the Ginkgo/Gomega framework for BDD-style testing.

Key features:
- Real Kubernetes cluster with kind
- Real PostgreSQL and MySQL databases
- LocalStack for AWS Secrets Manager emulation
- Full end-to-end testing of the operator
- Automatic cleanup between tests

### Running Integration Tests

```bash
# Complete workflow: setup cluster, run tests, teardown
make integration-test-all

# Or run steps individually:
make integration-test-setup    # Create cluster and deploy services
make integration-test           # Run the tests
make integration-test-teardown  # Clean up

# Rebuild operator during development
make integration-test-rebuild-operator
```

## Test Coverage

### Combined Coverage (Unit + Integration)

To get complete code coverage that includes both unit tests and integration tests:

```bash
# Run all tests with coverage and merge the results
make integration-test-all-coverage
```

This will:
1. Run unit tests and generate `cover.out`
2. Build the operator with coverage instrumentation (`-cover`)
3. Deploy it to the test cluster with `GOCOVERDIR=/tmp/coverage`
4. Run integration tests (coverage data is collected automatically)
5. Merge unit and integration coverage into `coverage/merged.out`
6. Display total coverage summary

**View coverage report:**
```bash
# Generate HTML coverage report
go tool cover -html=coverage/merged.out -o coverage/coverage.html

# View summary
go tool cover -func=coverage/merged.out
```

### Coverage Details

The coverage system works by:
- **Unit tests**: Standard `-coverprofile` flag generates `cover.out`
- **Integration tests**: Operator built with `-cover` writes coverage to `/tmp/coverage` (mounted via hostPath from kind)
- **Merging**: `go tool covdata textfmt` converts binary coverage data, then files are merged

**Coverage workflow diagram:**
```
Unit Tests → cover.out
                ↓
Integration Tests → /tmp/coverage/* (binary format)
                ↓
        go tool covdata textfmt → coverage/integration.out
                ↓
            Merge → coverage/merged.out
                ↓
        Total Coverage Report
```

## Running Tests

### Prerequisites

Before running integration tests, ensure you have the required tools installed:

```bash
# Check if all system dependencies are available
make check-integration-tools

# Install kind and helm (automatically uses Homebrew on macOS if available)
make integration-tools
```

**Required System Tools:**

On **macOS with Homebrew**, install prerequisites with:
```bash
brew install docker kubectl go
# or just: brew install --cask docker && brew install kubectl go
```

Otherwise, install manually:
- **Docker**: Download from https://docs.docker.com/get-docker/
- **kubectl**: Installation guide at https://kubernetes.io/docs/tasks/tools/
- **Go**: Download from https://golang.org/doc/install

**Auto-installed Tools:**

The `make integration-tools` command will automatically install:
- **kind**: Uses Homebrew on macOS, otherwise uses `go install`
- **helm**: Uses Homebrew on macOS, otherwise downloads from official releases

All tools are symlinked to `./bin/` directory for consistency.

### Run all unit tests

Unit tests are fast, require no external dependencies, and test individual functions in isolation:

```bash
make test
```

These tests include:
- Database connection string parsing (PostgreSQL, MySQL)
- Password generation and security
- SQL identifier and literal quoting (SQL injection protection)
- AWS region validation
- Secret data serialization
- Controller helper functions

**Important**: Unit tests do NOT require databases or external services. They run in under a second.

### Run integration tests

Integration tests run against a real Kubernetes cluster with PostgreSQL, MySQL, and LocalStack (AWS emulator).

```bash
# Complete workflow: setup cluster, run tests, teardown
make integration-test-all

# Or run steps individually:

# 1. Setup cluster (installs tools, creates cluster, deploys databases)
make integration-test-setup

# 2. Run integration tests
make integration-test

# 3. Teardown cluster
make integration-test-teardown
```

**Note**: Integration tests require Docker to be running.

### Rebuild operator during development

If you're iterating on code and want to test changes without recreating the entire cluster:

```bash
# Rebuild operator image and redeploy to existing cluster
make integration-test-rebuild-operator
```

### Run with coverage
```bash
go test ./... -coverprofile=cover.out
go tool cover -html=cover.out
```

### Run specific unit test
```bash
go test -v ./internal/database -run TestParseConnectionString
```

### Run unit tests with coverage
```bash
go test ./internal/... -coverprofile=cover.out
go tool cover -html=cover.out
```

### Run integration tests only
```bash
go test -v -tags=integration ./test/integration/...
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Run tests
        run: make test

      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          file: ./cover.out
```

## Best Practices Summary

### Unit Tests
1. ✅ Fast - run in under a second
2. ✅ No external dependencies (no databases, no AWS, no Kubernetes)
3. ✅ Test individual functions in isolation
4. ✅ Use table-driven tests for validation logic
5. ✅ Test both happy path AND error cases
6. ✅ Test SQL injection protection (quoting functions)

### Integration Tests
1. ✅ Use real Kubernetes cluster (kind)
2. ✅ Use real databases (PostgreSQL, MySQL)
3. ✅ Use LocalStack for AWS emulation
4. ✅ Test complete end-to-end workflows
5. ✅ Clean up resources between tests
6. ✅ Test finalizer deletion logic
7. ✅ Test retainOnDelete behavior
8. ✅ Test orphaned resource detection

### General
1. ✅ Run tests in CI/CD
2. ✅ Measure and improve code coverage
3. ✅ Keep unit tests fast (no I/O)
4. ✅ Keep integration tests comprehensive

## Common Pitfalls

### ❌ Don't: Add external dependencies to unit tests
```go
// Bad - unit test trying to connect to database
func TestParseConnectionString(t *testing.T) {
    db, _ := sql.Open("postgres", "postgresql://localhost/test")
    // This is an INTEGRATION test, not a unit test!
}
```

### ✅ Do: Keep unit tests pure
```go
// Good - pure function testing
func TestParseConnectionString(t *testing.T) {
    connInfo, err := ParseConnectionString("postgres://user:pass@localhost:5432/db")
    if err != nil {
        t.Fatal(err)
    }
    if connInfo.Host != "localhost" {
        t.Errorf("Expected localhost, got %s", connInfo.Host)
    }
}
```

## Resources

- [Kubebuilder Testing Guide](https://book.kubebuilder.io/cronjob-tutorial/writing-tests)
- [Operator SDK Testing](https://sdk.operatorframework.io/docs/building-operators/golang/testing/)
- [Ginkgo Documentation](https://onsi.github.io/ginkgo/) - BDD testing framework
- [Gomega Matchers](https://onsi.github.io/gomega/) - Assertion library
- [kind Documentation](https://kind.sigs.k8s.io/) - Kubernetes in Docker
- [Table-Driven Tests in Go](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
