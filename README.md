# Database User Operator

A Kubernetes operator that automates database and user creation for PostgreSQL, MySQL, and MariaDB with AWS Secrets Manager integration.

## Overview

The Database User Operator creates and manages databases and users declaratively through Kubernetes Custom Resources. It automatically:
- Creates databases and users with secure random passwords
- Stores credentials in AWS Secrets Manager
- Grants configurable privileges
- Handles resource lifecycle with configurable retention
- Supports PostgreSQL, MySQL, and MariaDB

## Quick Start

### Prerequisites

- Kubernetes cluster (v1.28+)
- PostgreSQL, MySQL, or MariaDB database instance
- AWS credentials with Secrets Manager permissions

### Installation

Install using Helm from the OCI registry:

```bash
# Install the latest version
helm install database-user-operator \
  oci://ghcr.io/opzkit/charts/database-user-operator \
  --namespace db-system \
  --create-namespace

# Or install a specific version
helm install database-user-operator \
  oci://ghcr.io/opzkit/charts/database-user-operator \
  --version 0.1.0 \
  --namespace db-system \
  --create-namespace

# With custom values
helm install database-user-operator \
  oci://ghcr.io/opzkit/charts/database-user-operator \
  --namespace db-system \
  --create-namespace \
  --set replicaCount=2 \
  --set controllerManager.resources.limits.memory=256Mi
```

To upgrade the operator:
```bash
helm upgrade database-user-operator \
  oci://ghcr.io/opzkit/charts/database-user-operator \
  --namespace db-system
```

To uninstall:
```bash
helm uninstall database-user-operator \
  --namespace db-system
```

### Create Your First Database

#### PostgreSQL

1. Create a Kubernetes Secret with your PostgreSQL admin connection:
```bash
kubectl create secret generic postgres-admin \
  --from-literal=connectionString='postgresql://admin:password@db.example.com:5432/postgres'
```

2. Create a Database resource:
```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: myapp-db
spec:
  engine: postgres
  databaseName: myapp_db
  connectionStringSecretRef:
    name: postgres-admin
  awsSecretsManager:
    region: us-east-1
    tags:
      Environment: production
```

3. Apply and check status:
```bash
kubectl apply -f database.yaml
kubectl get database myapp-db
```

The operator will:
- Create database `myapp_db` and user `myapp_db` in PostgreSQL
- Generate a secure password
- Store credentials in AWS Secrets Manager at `rds/postgres/myapp_db`
- Grant ALL privileges to the user

### Retrieve Credentials

```bash
aws secretsmanager get-secret-value \
  --secret-id rds/postgres/myapp_db \
  --query SecretString \
  --output text | jq
```

The secret contains:
```json
{
  "DB_HOST": "db.example.com",
  "DB_PORT": 5432,
  "DB_NAME": "myapp_db",
  "DB_USERNAME": "myapp_db",
  "DB_PASSWORD": "generated-secure-password",
  "POSTGRES_URL": "postgresql://myapp_db:password@db.example.com:5432/myapp_db"
}
```

#### MySQL

1. Create a Kubernetes Secret with your MySQL admin connection:
```bash
kubectl create secret generic mysql-admin \
  --from-literal=connectionString='mysql://admin:password@db.example.com:3306/mysql'
```

2. Create a Database resource:
```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: myapp-mysql-db
spec:
  engine: mysql
  databaseName: myapp_db
  connectionStringSecretRef:
    name: mysql-admin
  awsSecretsManager:
    region: us-east-1
    tags:
      Environment: production
```

3. Apply and check status:
```bash
kubectl apply -f database.yaml
kubectl get database myapp-mysql-db
```

The operator will:
- Create database `myapp_db` with UTF8MB4 encoding
- Create user `myapp_db@'%'` (accessible from any host)
- Generate a secure password
- Store credentials in AWS Secrets Manager at `rds/mysql/myapp_db`
- Grant ALL privileges on `myapp_db.*` to the user

The secret format for MySQL:
```json
{
  "DB_HOST": "db.example.com",
  "DB_PORT": 3306,
  "DB_NAME": "myapp_db",
  "DB_USERNAME": "myapp_db",
  "DB_PASSWORD": "generated-secure-password",
  "MYSQL_URL": "mysql://myapp_db:password@db.example.com:3306/myapp_db"
}
```

#### MariaDB

MariaDB uses the same configuration as MySQL since it's MySQL-compatible. Simply use `engine: mariadb` in your Database resource:

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: myapp-mariadb
spec:
  engine: mariadb  # Uses MySQL driver and protocol
  databaseName: myapp_db
  connectionStringSecretRef:
    name: mariadb-admin
  awsSecretsManager:
    region: us-east-1
```

The operator treats MariaDB identically to MySQL, using the same driver and SQL syntax. The stored secret will use the `MYSQL_URL` format for compatibility.

## Advanced Examples

### Custom Secret Templates

Customize the secret format to match your application's configuration needs. For complete documentation including all available template variables and additional examples, see the [Secret Templates Guide](docs/SECRET_TEMPLATES.md).

#### Spring Boot Application

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: spring-app-db
spec:
  engine: postgres
  databaseName: spring_app
  connectionStringSecretRef:
    name: postgres-admin
  awsSecretsManager:
    region: us-east-1
  secretTemplate: |
    {
      "spring.datasource.url": "jdbc:postgresql://{{.DBHost}}:{{.DBPort}}/{{.DBName}}",
      "spring.datasource.username": "{{.DBUsername}}",
      "spring.datasource.password": "{{.DBPassword}}",
      "spring.datasource.driver-class-name": "org.postgresql.Driver"
    }
```

#### Connection String Only

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: simple-app-db
spec:
  engine: mysql
  databaseName: simple_app
  connectionStringSecretRef:
    name: mysql-admin
  awsSecretsManager:
    region: us-east-1
  secretTemplate: |
    {
      "connectionString": "{{.DatabaseURL}}"
    }
```

### Custom Privileges

Restrict user permissions:

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: readonly-db
spec:
  engine: postgres
  databaseName: myapp_db
  username: readonly_user
  privileges:
    - SELECT
  connectionStringSecretRef:
    name: postgres-admin
  awsSecretsManager:
    region: us-east-1
```

### Custom Username and Secret Path

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: custom-app-db
spec:
  engine: postgres
  databaseName: myapp_db
  username: custom_user
  secretName: /custom/path/myapp-credentials
  connectionStringSecretRef:
    name: postgres-admin
  awsSecretsManager:
    region: us-east-1
    description: "Custom application database"
    tags:
      Team: platform
      CostCenter: engineering
```

### Delete Resources on CR Deletion

By default, databases and users are retained when the Database CR is deleted. To change this:

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: temp-db
spec:
  engine: postgres
  databaseName: temp_db
  retainOnDelete: false  # Delete database and user when CR is deleted
  connectionStringSecretRef:
    name: postgres-admin
  awsSecretsManager:
    region: us-east-1
```

For more advanced examples and secret template documentation, see [Secret Templates Guide](docs/SECRET_TEMPLATES.md).

## Documentation

- [Installation Guide](docs/INSTALLATION.md) - Detailed installation and configuration
- [Usage Guide](docs/USAGE.md) - Complete examples and field reference
- [Secret Templates](docs/SECRET_TEMPLATES.md) - Customize secret format for your application
- [AWS Credentials](docs/AWS_CREDENTIALS.md) - IAM permissions and setup
- [Troubleshooting](docs/TROUBLESHOOTING.md) - Common issues and solutions
- [Testing Guide](docs/TESTING_GUIDE.md) - Unit and integration testing with coverage
- [Development Guide](docs/DEVELOPMENT.md) - Contributing and local development

## Key Features

- **Multi-Database Support**: PostgreSQL, MySQL, and MariaDB
- **Idempotent Operations**: Safe to reconcile multiple times - checks resource existence
- **Secure by Default**: Generates 32-character random passwords
- **AWS Integration**: Native AWS Secrets Manager support with tagging
- **Flexible Configuration**: Customizable privileges, usernames, and secret paths
- **Custom Secret Templates**: Adapt secret format to match your application's configuration
- **Safe Deletion**: Configurable resource retention with `retainOnDelete` (default: true)
- **Smart Reconciliation**: Only creates missing resources, never resets passwords
- **Error Recovery**: Handles missing secrets and marked-for-deletion gracefully

## Resource Lifecycle

The operator handles three resources: **Database**, **User**, and **Secret**

| DB | User | Secret | Behavior |
|---|---|---|---|
| ✅ | ✅ | ✅ | Skip creation, verify and apply grants |
| ✅ or ✗ | ✅ or ✗ | ✗ | **ERROR** - Cannot recover password |
| ✗ | ✗ | ✗ | Create all with new password |
| ✗ | ✗ | ✅ | Create DB + User using password from secret |

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions welcome! See [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md) for guidelines.
