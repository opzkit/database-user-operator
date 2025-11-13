# Integration Tests

This directory contains integration tests for the database-user-operator.

## Overview

Integration tests run against a real Kubernetes cluster (using kind) with:
- PostgreSQL database
- MySQL database
- LocalStack (AWS Secrets Manager emulation)
- The database-user-operator deployed

## Running Tests

### Quick Start

```bash
# Complete workflow: setup, test, teardown (with coverage)
make integration-test

# Or run steps individually:
make integration-setup     # Create cluster with coverage enabled
make integration-run       # Run tests
make integration-teardown  # Clean up
```

**Note**: Coverage collection is always enabled for integration tests. The complete workflow automatically generates merged coverage data from both unit and integration tests in `coverage/merged.out`.

## Coverage Collection

### How It Works

Integration test coverage is collected by:

1. **Building with instrumentation**: The operator binary is built with `-cover` flag
2. **Setting GOCOVERDIR**: Environment variable tells Go where to write coverage data
3. **Mounting hostPath**: `/tmp/coverage` is mounted from host → kind node → operator pod
4. **Running tests**: As the operator runs, it writes coverage files to `/tmp/coverage`
5. **Merging**: Coverage from unit tests (`cover.out`) and integration tests are merged

### Architecture

```
┌─────────────────────────────────────────────────────┐
│ Host Machine                                         │
│                                                      │
│  /tmp/coverage/  ← Coverage files written here      │
│       ↑                                              │
│       │ hostPath mount                               │
└───────┼──────────────────────────────────────────────┘
        │
┌───────┼──────────────────────────────────────────────┐
│ Kind Container (control-plane)                       │
│       │                                              │
│  /tmp/coverage/  ← Mounted as hostPath               │
│       ↑                                              │
│       │ Kubernetes hostPath volume                   │
└───────┼──────────────────────────────────────────────┘
        │
┌───────┼──────────────────────────────────────────────┐
│ Operator Pod                                         │
│       │                                              │
│  /tmp/coverage/  ← Mounted as volume                 │
│       ↑                                              │
│       │ GOCOVERDIR=/tmp/coverage                     │
│       │                                              │
│  [operator binary with -cover]                       │
│       Writes: covcounters.*, covmeta.*               │
└──────────────────────────────────────────────────────┘
```

## Test Structure

- `integration_test.go`: Test suite setup with Ginkgo/Gomega
- `operator_test.go`: Actual test cases covering:
  - Database creation (PostgreSQL, MySQL)
  - Credential management
  - AWS Secrets Manager integration
  - Resource cleanup and finalizers
  - Error handling (orphaned resources, etc.)

## Environment Variables

- `CLUSTER_NAME`: Name of the kind cluster (default: `database-operator-test`)

## Files

- `kind-config.yaml`: Kind cluster configuration with port mappings and volume mounts
- `manifests/`: Kubernetes manifests for PostgreSQL, MySQL, LocalStack
- `scripts/`:
  - `setup-cluster.sh`: Creates kind cluster and deploys all services
  - `teardown-cluster.sh`: Deletes the kind cluster
  - `init-localstack.sh`: Initializes LocalStack with test secrets

## Tips

### Debugging Tests

```bash
# Keep cluster running after tests
make integration-setup
make integration-run
# Don't run teardown - inspect cluster manually
kubectl get pods -A
kubectl logs -n db-system deployment/database-user-operator

# When done:
make integration-teardown
```

### Rebuilding Operator

During development, rebuild and redeploy without recreating the cluster:

```bash
make integration-rebuild
```

### Checking Coverage

After running with coverage:

```bash
# View files collected
ls -lh /tmp/coverage/

# Convert to text format manually
go tool covdata textfmt -i=/tmp/coverage -o=coverage/integration.out

# View coverage for specific package
go tool cover -func=coverage/integration.out | grep controller
```
