# Troubleshooting Guide

## AWS Credentials Issues

### Error: "The security token included in the request is invalid"

This error occurs when the operator tries to access AWS Secrets Manager but doesn't have valid AWS credentials.

**Important:** The operator ALWAYS needs AWS permissions because created database credentials are ALWAYS stored in AWS Secrets Manager, regardless of where you store the admin connection string.

#### Step 1: Verify the pod has AWS credentials

The AWS SDK uses a credential chain and will automatically discover credentials. Test if ANY credentials are available:

```bash
# Get the pod name
POD=$(kubectl get pods -l control-plane=controller-manager -o jsonpath='{.items[0].metadata.name}')

# Check if AWS credentials work (any method: IRSA, env vars, instance profile)
kubectl exec $POD -c manager -- aws sts get-caller-identity
```

If this command succeeds, the pod has working AWS credentials. The output shows which IAM principal is being used.

If it fails, the pod has NO AWS credentials configured.

#### Step 2: Identify which authentication method should be used

Check your deployment configuration:

**For IRSA (EKS):**
```bash
# Check if service account has role annotation
kubectl get serviceaccount database-user-operator \
  -o jsonpath='{.metadata.annotations.eks\.amazonaws\.com/role-arn}'
```

Should show: `arn:aws:iam::123456789012:role/your-role-name`

**For static credentials:**
```bash
# Check environment variables
kubectl exec $POD -c manager -- env | grep AWS
```

Should show: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`

**For EC2 instance profile:**
```bash
# Check if running on EC2 and metadata service is accessible
kubectl exec $POD -c manager -- curl -s http://169.254.169.254/latest/meta-data/iam/security-credentials/
```

#### Step 3: Test the credentials

Test if the credentials actually work:

```bash
# Get a shell in the operator pod
kubectl exec -it <pod-name> -c manager -- sh

# Test AWS credentials
aws sts get-caller-identity

# If that works, test Secrets Manager access
aws secretsmanager list-secrets --region us-east-1 --max-items 1
```

#### Step 4: Common Issues

**Issue 1: Secret not created**
```bash
# Create the secret with actual credentials
kubectl create secret generic aws-credentials \
  --from-literal=AWS_ACCESS_KEY_ID='AKIA...' \
  --from-literal=AWS_SECRET_ACCESS_KEY='...' \
  --from-literal=AWS_REGION='us-east-1'
```

**Issue 2: Secret in wrong namespace**

The secret must be in the same namespace as the operator:
```bash
kubectl get secret aws-credentials -n db-system
```

**Issue 3: Invalid credentials**

The credentials might be:
- Expired or rotated
- From wrong AWS account
- Lacking required permissions

Test with AWS CLI:
```bash
# Set the credentials locally
export AWS_ACCESS_KEY_ID='AKIA...'
export AWS_SECRET_ACCESS_KEY='...'
export AWS_REGION='us-east-1'

# Test authentication
aws sts get-caller-identity

# Test Secrets Manager access
aws secretsmanager create-secret \
  --name test-secret \
  --secret-string '{"test":"value"}' \
  --region us-east-1

# Clean up
aws secretsmanager delete-secret \
  --secret-id test-secret \
  --force-delete-without-recovery \
  --region us-east-1
```

#### Step 5: Enable debug logging

Redeploy with increased verbosity:
```bash
helm upgrade database-user-operator ./helm/database-user-operator \
  --set controllerManager.args[0]="--health-probe-bind-address=:8081" \
  --set controllerManager.args[1]="--metrics-bind-address=127.0.0.1:8080" \
  --set controllerManager.args[2]="--leader-elect" \
  --set controllerManager.args[3]="--zap-log-level=debug"
```

Check logs for more details:
```bash
kubectl logs -l control-plane=controller-manager -c manager --tail=100
```

## Connection String Issues

### Error: "unsupported connection string format"

The connection string must be in PostgreSQL URL format:
```
postgresql://username:password@host:port/database?sslmode=require
```

or

```
postgres://username:password@host:port/database?sslmode=require
```

### Error: "failed to ping database"

**Check 1: Network connectivity**
```bash
# From the operator pod
kubectl exec <pod-name> -c manager -- nc -zv <postgres-host> 5432
```

**Check 2: Credentials**
Verify the admin connection string has valid credentials.

**Check 3: SSL mode**
If database requires SSL, ensure connection string includes `?sslmode=require`

## Database Resource Not Reconciling

### Check the status

```bash
kubectl get database myapp-database -o yaml
```

Look at:
- `status.phase`: Should be "Ready" or "Error"
- `status.message`: Contains error details
- `status.observedGeneration`: Should match `metadata.generation`

### Check events

```bash
kubectl describe database myapp-database
```

Look for Warning events at the bottom.

### Check operator logs

```bash
kubectl logs -l control-plane=controller-manager -c manager --tail=100 -f
```

## Credential Storage Backend

**Created database credentials are ALWAYS stored in AWS Secrets Manager.**

The connection string source only determines where to read the ADMIN connection string:

| Connection String Source | Admin Connection From | Created Credentials Stored In |
|-------------------------|----------------------|------------------------------|
| `connectionStringSecretRef` | Kubernetes Secret | AWS Secrets Manager ⚠️ |
| `connectionStringAWSSecretRef` | AWS Secrets Manager | AWS Secrets Manager |

**This means:** AWS permissions are ALWAYS required, even if you use Kubernetes secrets for the admin connection.

### Verify which backend is being used

```bash
kubectl get database myapp-database -o jsonpath='{.spec}' | jq
```

Look for either:
- `connectionStringSecretRef`: Using Kubernetes
- `connectionStringAWSSecretRef`: Using AWS

## Rate Limiting / Exponential Backoff

The operator uses exponential backoff for errors:
- 1st retry: 5 seconds
- 2nd retry: 10 seconds
- 3rd retry: 20 seconds
- 4th retry: 40 seconds
- 5th+ retry: 60 seconds (max)

After a successful reconciliation, the backoff resets.

To force immediate retry, update the spec:
```bash
kubectl annotate database myapp-database force-sync="$(date +%s)" --overwrite
```
