# Usage Guide

Complete guide to using the Database User Operator.

## Table of Contents
- [Basic Usage](#basic-usage)
- [Field Reference](#field-reference)
- [Examples](#examples)
- [Secret Format](#secret-format)
- [Resource Lifecycle](#resource-lifecycle)
- [kubectl Commands](#kubectl-commands)

## Basic Usage

###

 Minimal Example

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
```

This creates:
- Database: `myapp_db`
- User: `myapp_db` (defaults to database name)
- Secret: `rds/postgres/myapp_db` (default path)
- Privileges: `ALL` (default)
- Retention: `true` (default - resources retained on deletion)

## Field Reference

### Spec Fields

#### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `engine` | string | Database engine: `postgres`, `postgresql`, `mysql`, `mariadb` |
| `databaseName` | string | Name of database to create (pattern: `^[a-z][a-z0-9_]*$`, max 63 chars) |
| `connectionStringSecretRef` OR `connectionStringAWSSecretRef` | object | Admin connection string reference |

#### Optional Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `username` | string | `databaseName` | Username for created user |
| `secretName` | string | `rds/<engine>/<databaseName>` | AWS secret path |
| `privileges` | []string | `["ALL"]` | Privileges to grant |
| `retainOnDelete` | bool | `true` | Retain resources on CR deletion |
| `awsSecretsManager` | object | - | AWS Secrets Manager config |

### connectionStringSecretRef

Reference to Kubernetes Secret containing admin connection string:

```yaml
connectionStringSecretRef:
  name: postgres-admin          # required
  key: connectionString          # optional, defaults to "connectionString"
```

The referenced secret should contain:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: postgres-admin
stringData:
  connectionString: "postgresql://admin:password@db.example.com:5432/postgres?sslmode=require"
```

### connectionStringAWSSecretRef

Reference to AWS Secrets Manager secret containing admin connection string:

```yaml
connectionStringAWSSecretRef:
  secretName: rds/admin/postgres    # required - name or ARN
  key: connectionString               # optional, defaults to "connectionString"
  region: us-east-1                   # optional, uses AWS SDK default if not specified
```

### awsSecretsManager

Configuration for storing created credentials:

```yaml
awsSecretsManager:
  region: us-east-1                   # optional, defaults to AWS SDK default
  description: "DB credentials"       # optional
  tags:                               # optional
    Environment: production
    Application: myapp
    ManagedBy: database-user-operator
```

**Note**: Created credentials are **always** stored in AWS Secrets Manager, regardless of where the admin connection string comes from.

### Region Priority

The operator determines AWS region in this order:
1. `spec.awsSecretsManager.region` (highest priority)
2. `spec.connectionStringAWSSecretRef.region`
3. AWS SDK default (environment variables, instance metadata, etc.)

### Privileges

Grant specific privileges or use ALL:

```yaml
# Grant all privileges (default)
privileges:
  - ALL

# Grant specific privileges
privileges:
  - SELECT
  - INSERT
  - UPDATE
  - DELETE

# Read-only access
privileges:
  - SELECT
```

See [PostgreSQL GRANT documentation](https://www.postgresql.org/docs/current/sql-grant.html) for available privileges.

## Examples

### Example 1: Basic PostgreSQL Database

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: app-db
  namespace: production
spec:
  engine: postgres
  databaseName: app_database
  connectionStringSecretRef:
    name: postgres-admin-connection
  awsSecretsManager:
    region: us-east-1
    tags:
      Environment: production
```

### Example 2: Custom Username and Secret Path

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: analytics-db
spec:
  engine: postgres
  databaseName: analytics
  username: analytics_user
  secretName: /myapp/databases/analytics
  connectionStringSecretRef:
    name: postgres-admin
  privileges:
    - SELECT
    - INSERT
```

### Example 3: Using AWS Secret for Admin Connection

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: api-db
spec:
  engine: postgresql
  databaseName: api_db
  connectionStringAWSSecretRef:
    secretName: rds/admin/postgres-main
    region: eu-west-1
  awsSecretsManager:
    region: eu-west-1
    description: "API database credentials"
    tags:
      Team: backend
      Service: api
```

### Example 4: Temporary Database (Cleanup on Deletion)

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: test-db
  namespace: testing
spec:
  engine: postgres
  databaseName: test_temp
  retainOnDelete: false  # Delete all resources on CR deletion
  connectionStringSecretRef:
    name: postgres-admin
```

### Example 5: Read-Only User

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: readonly-db
spec:
  engine: postgres
  databaseName: reports
  username: readonly_user
  privileges:
    - SELECT
  connectionStringSecretRef:
    name: postgres-admin
```

### Example 6: MySQL Database

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: myapp-mysql
spec:
  engine: mysql
  databaseName: myapp_db
  username: myapp_user
  connectionStringSecretRef:
    name: mysql-admin
  awsSecretsManager:
    region: us-east-1
    tags:
      Environment: production
```

### Example 7: MariaDB with Custom Secret Path

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: analytics-mariadb
spec:
  engine: mariadb
  databaseName: analytics_db
  secretName: /mariadb/production/analytics
  connectionStringAWSSecretRef:
    secretName: rds/admin/mariadb-main
    key: connectionString
    region: eu-west-1
```

## Secret Format

Created secrets in AWS Secrets Manager have this format:

**PostgreSQL:**
```json
{
  "DB_HOST": "db.example.com",
  "DB_PORT": 5432,
  "DB_NAME": "myapp_db",
  "DB_USERNAME": "myapp_user",
  "DB_PASSWORD": "generated-32-char-secure-password",
  "POSTGRES_URL": "postgresql://myapp_user:password@db.example.com:5432/myapp_db"
}
```

**MySQL/MariaDB:**
```json
{
  "DB_HOST": "db.example.com",
  "DB_PORT": 3306,
  "DB_NAME": "myapp_db",
  "DB_USERNAME": "myapp_user",
  "DB_PASSWORD": "generated-32-char-secure-password",
  "MYSQL_URL": "mysql://myapp_user:password@db.example.com:3306/myapp_db"
}
```

### Field Descriptions

| Field | Description |
|-------|-------------|
| `DB_HOST` | Database host |
| `DB_PORT` | Database port |
| `DB_NAME` | Database name |
| `DB_USERNAME` | Username |
| `DB_PASSWORD` | Generated password (32 characters, base64-encoded random) |
| `POSTGRES_URL` or `MYSQL_URL` | Full connection URL (engine-specific) |

### Retrieving Secrets

**Using AWS CLI:**
```bash
aws secretsmanager get-secret-value \
  --secret-id rds/postgres/myapp_db \
  --query SecretString \
  --output text | jq
```

**Using AWS SDK (Python):**
```python
import boto3
import json

client = boto3.client('secretsmanager')
response = client.get_secret_value(SecretId='rds/postgres/myapp_db')
credentials = json.loads(response['SecretString'])

print(f"Connection: {credentials['POSTGRES_URL']}")
```

**From Application (using environment variables):**
```bash
# Export secret fields as environment variables
aws secretsmanager get-secret-value \
  --secret-id rds/postgres/myapp_db \
  --query SecretString \
  --output text | jq -r 'to_entries|map("\(.key)=\(.value|tostring)")|.[]'
```

## Resource Lifecycle

### Creation Flow

When you create a Database resource, the operator:

1. **Checks actual existence** in PostgreSQL and AWS:
   - Does database exist in PostgreSQL?
   - Does user exist in PostgreSQL?
   - Does secret exist in AWS Secrets Manager?

2. **Takes action based on state**:

   | Database | User | Secret | Action |
   |----------|------|--------|--------|
   | ✅ | ✅ | ✅ | Skip creation, apply grants, update status |
   | ✅ or ✗ | ✅ or ✗ | ✗ | **ERROR** - Cannot recover password |
   | ✗ | ✗ | ✗ | Create all with new password |
   | ✗ | ✗ | ✅ | Create DB + User with password from secret |

3. **Grant privileges** on the database

4. **Update status** with created resource information

### Status Fields

```yaml
status:
  phase: Ready                        # Pending, Creating, Ready, Error, Deleting
  message: "Database, user, and secret are ready"
  conditions: [...]
  observedGeneration: 1

  # Resource tracking
  databaseCreated: true
  userCreated: true
  secretCreated: true

  # Created resource details
  actualUsername: myapp_db
  actualSecretName: rds/postgres/myapp_db
  secretARN: arn:aws:secretsmanager:us-east-1:123456789012:secret:rds/postgres/myapp_db-abcdef
  secretVersion: v2
  secretFormatVersion: v2

  # Connection info (non-sensitive)
  connectionInfo:
    host: db.example.com
    port: 5432
    database: myapp_db
    username: myapp_db
    engine: postgres
```

### Deletion Behavior

#### With `retainOnDelete: true` (default)

```bash
kubectl delete database myapp-db
```

The operator:
- Removes the Kubernetes Database resource
- **Keeps** the PostgreSQL database
- **Keeps** the PostgreSQL user
- **Keeps** the AWS Secrets Manager secret

Use this for production databases where data should survive CR deletion.

#### With `retainOnDelete: false`

```bash
kubectl delete database myapp-db
```

The operator:
- Drops the PostgreSQL database
- Drops the PostgreSQL user
- Deletes the AWS Secrets Manager secret (with 7-day recovery window)
- Removes the Kubernetes Database resource

Use this for temporary/test databases.

### Updating Resources

#### What triggers reconciliation?
- Creating a new Database resource
- Updating Database spec fields
- Secret format version mismatch (automatic migration)
- Operator restart (idempotent checks prevent duplicates)

#### What operations are safe?
- ✅ Updating `awsSecretsManager.tags` - Only updates secret tags
- ✅ Updating `awsSecretsManager.description` - Only updates description
- ✅ Updating `privileges` - Reapplies grants
- ❌ Changing `databaseName` - Not supported (create new resource)
- ❌ Changing `username` - Not supported (create new resource)
- ❌ Changing `engine` - Not supported (create new resource)

#### Password Management

**Passwords are NEVER changed after initial creation** unless:
- Creating a new user (doesn't exist yet)
- Secret was deleted externally (recovery scenario)

The operator **never resets passwords** on existing users, even when:
- Updating CRD tags
- Reapplying grants
- Operator restarts

## kubectl Commands

### View Databases

```bash
# List all databases
kubectl get databases
kubectl get db  # short name

# List in specific namespace
kubectl get databases -n production

# List across all namespaces
kubectl get databases --all-namespaces

# Watch for changes
kubectl get databases --watch
```

Output:
```
NAME        ENGINE     DATABASE    USERNAME    SECRETNAME                  PHASE   AGE
myapp-db    postgres   myapp_db    myapp_db    rds/postgres/myapp_db      Ready   5m
```

### Describe Database

```bash
kubectl describe database myapp-db
```

Shows:
- Full spec configuration
- Status and conditions
- Events (creation, errors, etc.)
- Connection information

### View Events

```bash
kubectl get events --field-selector involvedObject.kind=Database
```

### View Logs

```bash
# Follow operator logs
kubectl logs -n database-user-operator-system \
  deployment/database-user-operator-controller-manager \
  -c manager \
  --follow

# Filter for specific database
kubectl logs -n database-user-operator-system \
  deployment/database-user-operator-controller-manager \
  -c manager | grep "myapp-db"
```

### Update Database

```bash
# Edit interactively
kubectl edit database myapp-db

# Update from file
kubectl apply -f database.yaml

# Patch specific field
kubectl patch database myapp-db \
  --type merge \
  -p '{"spec":{"privileges":["SELECT","INSERT"]}}'
```

### Delete Database

```bash
# Delete (respects retainOnDelete setting)
kubectl delete database myapp-db

# Delete immediately without waiting
kubectl delete database myapp-db --wait=false

# Force delete if stuck
kubectl patch database myapp-db \
  -p '{"metadata":{"finalizers":[]}}' \
  --type=merge
kubectl delete database myapp-db
```

## Engine-Specific Notes

### PostgreSQL

**Admin User Requirements:**
- `CREATEDB` privilege - to create databases
- `CREATEROLE` privilege - to create users

**Connection String Formats:**
```
postgresql://user:password@host:port/database?sslmode=require
postgres://user:password@host:port/database
```

**SSL Modes:**
- `sslmode=require` - Recommended for RDS and production
- `sslmode=disable` - Local development only
- `sslmode=prefer`, `sslmode=verify-ca`, `sslmode=verify-full` - Various verification levels

**Privileges:** Supports all PostgreSQL database-level privileges (SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER, CREATE, CONNECT, TEMPORARY, ALL)

**Secret Field:** Credentials stored with `POSTGRES_URL` field

### MySQL / MariaDB

**Admin User Requirements:**
- `CREATE` privilege - to create databases
- `CREATE USER` privilege - to create users
- `GRANT OPTION` - to grant privileges

**Connection String Formats:**
```
mysql://user:password@host:port/database
user:password@tcp(host:port)/database
```

**Character Set:** Databases created with `CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci`

**User Host:** Users created with `'username'@'%'` (accessible from any host)

**Privileges:** Supports MySQL privileges (SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, ALTER, INDEX, REFERENCES, ALL)

**Secret Field:** Credentials stored with `MYSQL_URL` field

**MariaDB Note:** MariaDB uses identical configuration to MySQL (same driver and protocol)

### AWS RDS Considerations

**PostgreSQL RDS:**
- Use master user or user with `rds_superuser` role
- Always use `sslmode=require` for security
- Ensure security group allows operator pod access

**MySQL/MariaDB RDS:**
- Use master user credentials
- Enable `require_secure_transport` for SSL
- Ensure security group allows operator pod access

## Next Steps

- [AWS Credentials](AWS_CREDENTIALS.md) - Set up IAM permissions
- [Installation Guide](INSTALLATION.md) - Detailed installation and configuration
- [Troubleshooting](TROUBLESHOOTING.md) - Common issues and solutions
