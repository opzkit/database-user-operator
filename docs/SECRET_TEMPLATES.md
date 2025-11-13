# Secret Templates

The Database User Operator allows you to customize the structure of secrets stored in AWS Secrets Manager using Go templates. This enables you to adapt the secret format to match your application's requirements.

## Overview

By default, the operator stores secrets in the following format:

```json
{
  "DB_HOST": "db.example.com",
  "DB_PORT": 5432,
  "DB_NAME": "myapp_db",
  "DB_USERNAME": "myapp_db",
  "DB_PASSWORD": "generated-password",
  "POSTGRES_URL": "postgresql://myapp_db:password@db.example.com:5432/myapp_db"
}
```

With secret templates, you can customize this format to match your application's configuration requirements.

## Template Variables

The following variables are available in your template:

| Variable | Type | Description | Example |
|----------|------|-------------|---------|
| `.DBHost` | string | Database host | `db.example.com` |
| `.DBPort` | int | Database port | `5432` |
| `.DBName` | string | Database name | `myapp_db` |
| `.DBUsername` | string | Database username | `myapp_user` |
| `.DBPassword` | string | Generated password | `aB3$...` |
| `.DatabaseURL` | string | Full database URL | `postgresql://...` |
| `.Engine` | string | Database engine | `postgres`, `mysql` |

## Template Syntax

Templates use [Go template syntax](https://pkg.go.dev/text/template). The most common operations:

- **Variable substitution**: `{{.DBHost}}`
- **Numeric values**: `{{.DBPort}}` (no quotes for numbers)
- **String values**: `"{{.DBHost}}"` (quotes for strings)

**Important**: Templates must produce valid JSON output.

## Examples

### Default Template (Implicit)

If you don't specify a `secretTemplate`, this is what the operator uses:

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
  # No secretTemplate specified - uses default format
```

Result:
```json
{
  "DB_HOST": "db.example.com",
  "DB_PORT": 5432,
  "DB_NAME": "myapp_db",
  "DB_USERNAME": "myapp_db",
  "DB_PASSWORD": "...",
  "POSTGRES_URL": "postgresql://..."
}
```

### Spring Boot Application

For Spring Boot applications that expect specific property names:

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

Result:
```json
{
  "spring.datasource.url": "jdbc:postgresql://db.example.com:5432/spring_app",
  "spring.datasource.username": "spring_app",
  "spring.datasource.password": "...",
  "spring.datasource.driver-class-name": "org.postgresql.Driver"
}
```

### Connection String Only

For applications that only need a single connection string:

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

Result:
```json
{
  "connectionString": "mysql://simple_app:password@db.example.com:3306/simple_app"
}
```

### Environment Variable Style

For applications using environment variables with specific naming:

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: env-app-db
spec:
  engine: postgres
  databaseName: env_app
  connectionStringSecretRef:
    name: postgres-admin
  awsSecretsManager:
    region: us-east-1
  secretTemplate: |
    {
      "DATABASE_HOST": "{{.DBHost}}",
      "DATABASE_PORT": "{{.DBPort}}",
      "DATABASE_NAME": "{{.DBName}}",
      "DATABASE_USER": "{{.DBUsername}}",
      "DATABASE_PASSWORD": "{{.DBPassword}}",
      "DATABASE_URL": "{{.DatabaseURL}}"
    }
```

Result:
```json
{
  "DATABASE_HOST": "db.example.com",
  "DATABASE_PORT": "5432",
  "DATABASE_NAME": "env_app",
  "DATABASE_USER": "env_app",
  "DATABASE_PASSWORD": "...",
  "DATABASE_URL": "postgresql://..."
}
```

### Django Application

For Django applications:

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: django-app-db
spec:
  engine: postgres
  databaseName: django_app
  connectionStringSecretRef:
    name: postgres-admin
  awsSecretsManager:
    region: us-east-1
  secretTemplate: |
    {
      "ENGINE": "django.db.backends.postgresql",
      "NAME": "{{.DBName}}",
      "USER": "{{.DBUsername}}",
      "PASSWORD": "{{.DBPassword}}",
      "HOST": "{{.DBHost}}",
      "PORT": "{{.DBPort}}"
    }
```

Result:
```json
{
  "ENGINE": "django.db.backends.postgresql",
  "NAME": "django_app",
  "USER": "django_app",
  "PASSWORD": "...",
  "HOST": "db.example.com",
  "PORT": "5432"
}
```

### Node.js/TypeORM Application

For Node.js applications using TypeORM:

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: nodejs-app-db
spec:
  engine: mysql
  databaseName: nodejs_app
  connectionStringSecretRef:
    name: mysql-admin
  awsSecretsManager:
    region: us-east-1
  secretTemplate: |
    {
      "type": "mysql",
      "host": "{{.DBHost}}",
      "port": {{.DBPort}},
      "username": "{{.DBUsername}}",
      "password": "{{.DBPassword}}",
      "database": "{{.DBName}}"
    }
```

Result:
```json
{
  "type": "mysql",
  "host": "db.example.com",
  "port": 3306,
  "username": "nodejs_app",
  "password": "...",
  "database": "nodejs_app"
}
```

## Best Practices

### 1. Always Produce Valid JSON

Templates must generate valid JSON. The operator validates the output before storing it.

**Bad** (missing quotes):
```yaml
secretTemplate: |
  {
    "host": {{.DBHost}}  # ERROR: DBHost is a string, needs quotes
  }
```

**Good**:
```yaml
secretTemplate: |
  {
    "host": "{{.DBHost}}"
  }
```

### 2. Numbers Don't Need Quotes

Port numbers are integers and should not be quoted in JSON:

**Bad**:
```yaml
secretTemplate: |
  {
    "port": "{{.DBPort}}"  # Produces "5432" (string) instead of 5432 (number)
  }
```

**Good**:
```yaml
secretTemplate: |
  {
    "port": {{.DBPort}}  # Produces 5432 (number)
  }
```

### 3. Use Multiline Strings for Readability

YAML supports multiline strings with `|`:

```yaml
secretTemplate: |
  {
    "field1": "{{.DBHost}}",
    "field2": {{.DBPort}},
    "field3": "{{.DBUsername}}"
  }
```

### 4. Test Your Templates

Before deploying, test your template locally or in a development environment to ensure it produces the expected JSON structure.

### 5. Document Your Custom Template

Add comments in your Database resource to explain why you're using a custom template:

```yaml
apiVersion: database.opzkit.io/v1alpha1
kind: Database
metadata:
  name: myapp-db
spec:
  engine: postgres
  databaseName: myapp
  connectionStringSecretRef:
    name: postgres-admin
  awsSecretsManager:
    region: us-east-1
    description: "Database credentials for Spring Boot application"
    tags:
      Application: myapp
      Framework: spring-boot
  # Custom template for Spring Boot application.properties format
  secretTemplate: |
    {
      "spring.datasource.url": "jdbc:postgresql://{{.DBHost}}:{{.DBPort}}/{{.DBName}}",
      "spring.datasource.username": "{{.DBUsername}}",
      "spring.datasource.password": "{{.DBPassword}}"
    }
```

## Error Handling

### Template Parse Errors

If your template has syntax errors, the operator will report an error:

```
failed to parse secret template: template: secret:1: unexpected "}" in operand
```

**Fix**: Check your template syntax, especially matching brackets and quote marks.

### Invalid JSON Output

If your template produces invalid JSON, the operator will report:

```
template output is not valid JSON: invalid character '}' looking for beginning of value
```

**Fix**: Ensure your template produces valid JSON. Common issues:
- Missing or extra commas
- Unquoted string values
- Extra closing braces

### Undefined Variables

If you reference a variable that doesn't exist:

```
failed to execute secret template: template: secret:1:15: executing "secret" at <.InvalidField>: can't evaluate field InvalidField
```

**Fix**: Use only the documented template variables listed above.

## Migration from Default Format

If you have existing secrets and want to migrate to a custom template:

1. **Create a new Database resource** with the custom template
2. **Update your application** to use the new secret format
3. **Delete the old Database resource** (if `retainOnDelete: false`)

The operator will create a new secret with the custom format. Existing applications can continue using the old secret until migrated.

## Limitations

1. Templates must produce valid JSON (no YAML, TOML, or other formats)
2. Template execution is synchronous and must complete quickly
3. No support for conditional logic or loops (templates should be simple field mappings)
4. Cannot access external data or make API calls from templates

## Related Documentation

- [Usage Guide](USAGE.md) - General usage and field reference
- [AWS Credentials](AWS_CREDENTIALS.md) - AWS Secrets Manager setup
- [Examples](../config/samples/) - Sample Database resources

## Support

If you encounter issues with secret templates:

1. Check the operator logs for detailed error messages
2. Validate your JSON output using a JSON validator
3. Review the template examples in this guide
4. Open an issue on GitHub with your template and error message
