#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

LOCALSTACK_ENDPOINT="${LOCALSTACK_ENDPOINT:-http://localhost:14566}"
AWS_REGION="${AWS_REGION:-us-east-1}"

export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=$AWS_REGION
export AWS_PAGER=""  # Disable pager for CI compatibility

echo "===> Initializing LocalStack Secrets Manager"

# Wait for LocalStack to be ready
echo "Waiting for LocalStack to be ready..."
LOCALSTACK_READY=false
for i in {1..30}; do
    if curl -sf "${LOCALSTACK_ENDPOINT}/_localstack/health" > /dev/null 2>&1; then
        HEALTH_OUTPUT=$(curl -s "${LOCALSTACK_ENDPOINT}/_localstack/health")
        # Accept both "running" and "available" as valid ready states
        if echo "$HEALTH_OUTPUT" | grep -qE '"secretsmanager"[[:space:]]*:[[:space:]]*"(running|available)"'; then
            echo "✓ LocalStack is ready! (attempt $i/30)"
            LOCALSTACK_READY=true
            break
        else
            # Show why it's not ready on first few attempts
            if [ $i -le 3 ] || [ $((i % 5)) -eq 0 ]; then
                SM_STATUS=$(echo "$HEALTH_OUTPUT" | grep -o '"secretsmanager"[^,}]*' || echo "secretsmanager not found")
                echo "Waiting for LocalStack... (attempt $i/30) - Status: $SM_STATUS"
            else
                echo "Waiting for LocalStack... (attempt $i/30)"
            fi
        fi
    else
        echo "Waiting for LocalStack... (attempt $i/30) - endpoint not reachable"
    fi
    sleep 2
done

if [ "$LOCALSTACK_READY" = false ]; then
    echo "ERROR: LocalStack failed to become ready after 30 attempts"
    echo "Last health check output:"
    curl -s "${LOCALSTACK_ENDPOINT}/_localstack/health" || echo "Failed to fetch health status"
    exit 1
fi

# Create PostgreSQL connection string secret
echo "Creating PostgreSQL connection secret..."
aws secretsmanager create-secret \
    --endpoint-url="${LOCALSTACK_ENDPOINT}" \
    --region="${AWS_REGION}" \
    --name "test/postgres/connection" \
    --description "PostgreSQL connection string for integration tests" \
    --secret-string "postgres://postgres:password@postgres.databases.svc.cluster.local:5432/postgres?sslmode=disable" \
    || echo "Secret test/postgres/connection already exists"

# Create MySQL connection string secret
echo "Creating MySQL connection secret..."
aws secretsmanager create-secret \
    --endpoint-url="${LOCALSTACK_ENDPOINT}" \
    --region="${AWS_REGION}" \
    --name "test/mysql/connection" \
    --description "MySQL connection string for integration tests" \
    --secret-string "mysql://root:password@mysql.databases.svc.cluster.local:3306/" \
    || echo "Secret test/mysql/connection already exists"

# Verify secrets were created
echo ""
echo "Verifying secrets..."
SECRET_COUNT=$(aws secretsmanager list-secrets \
    --endpoint-url="${LOCALSTACK_ENDPOINT}" \
    --region="${AWS_REGION}" \
    --query 'length(SecretList)' \
    --output text)

if [ "$SECRET_COUNT" -ge 2 ]; then
    echo "✓ Successfully created $SECRET_COUNT secrets"
    aws secretsmanager list-secrets \
        --endpoint-url="${LOCALSTACK_ENDPOINT}" \
        --region="${AWS_REGION}" \
        --query 'SecretList[*].[Name,Description]' \
        --output text
else
    echo "⚠ Warning: Expected 2 secrets but found $SECRET_COUNT"
fi

echo ""
echo "===> LocalStack initialization complete!"
