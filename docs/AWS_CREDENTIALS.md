# AWS Credentials Configuration

The database-user-operator requires AWS permissions to store created database credentials in AWS Secrets Manager. The operator uses the standard AWS SDK credential chain, which automatically tries multiple authentication methods in order.

**Important:** Created database credentials are ALWAYS stored in AWS Secrets Manager, regardless of whether you store the admin connection string in Kubernetes or AWS.

## Authentication Methods

The AWS SDK automatically discovers credentials using this priority order:
1. Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
2. Web Identity Token (IRSA for EKS)
3. Shared credentials file
4. EC2 Instance Profile
5. ECS Task Role

You can use any of these methods - configure the one that works best for your environment.

### 1. IRSA (Recommended for EKS)

IAM Roles for Service Accounts is the recommended approach for EKS clusters.

**Helm Configuration:**
```yaml
serviceAccount:
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/database-operator-role
```

**Required IAM Policy:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "secretsmanager:CreateSecret",
        "secretsmanager:UpdateSecret",
        "secretsmanager:DeleteSecret",
        "secretsmanager:DescribeSecret",
        "secretsmanager:GetSecretValue",
        "secretsmanager:PutSecretValue",
        "secretsmanager:TagResource"
      ],
      "Resource": "*"
    }
  ]
}
```

### 2. Static Credentials (Kubernetes Secret)

**Not recommended for production** - use IRSA or EC2 instance profiles instead.

**Create Kubernetes Secret:**
```bash
kubectl create secret generic aws-credentials \
  --from-literal=AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE \
  --from-literal=AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY \
  --from-literal=AWS_REGION=us-east-1 \
  --namespace db-system
```

**Helm Configuration:**
```yaml
env:
  - name: AWS_ACCESS_KEY_ID
    valueFrom:
      secretKeyRef:
        name: aws-credentials
        key: AWS_ACCESS_KEY_ID
  - name: AWS_SECRET_ACCESS_KEY
    valueFrom:
      secretKeyRef:
        name: aws-credentials
        key: AWS_SECRET_ACCESS_KEY
  - name: AWS_REGION
    valueFrom:
      secretKeyRef:
        name: aws-credentials
        key: AWS_REGION
```

### 3. Environment Variables (Hardcoded)

**Not recommended** - credentials are visible in Helm values and pod specs.

**Helm Configuration:**
```yaml
env:
  - name: AWS_ACCESS_KEY_ID
    value: "AKIAIOSFODNN7EXAMPLE"
  - name: AWS_SECRET_ACCESS_KEY
    value: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
  - name: AWS_REGION
    value: "us-east-1"
```

### 4. EC2 Instance Profile

If running on EC2 (including EKS managed node groups), the operator automatically uses the instance profile attached to the EC2 instances.

**No Helm configuration needed** - works automatically if:
- EC2 instances have an IAM instance profile attached
- The instance profile has the required Secrets Manager permissions

This is a good option for self-managed Kubernetes clusters on EC2.

## Troubleshooting

### Error: "The security token included in the request is invalid"

**Possible Causes:**

1. **No AWS Credentials Available**
   - The pod doesn't have any AWS credentials configured
   - No IRSA role attached to the service account
   - No static credentials in environment variables
   - No EC2 instance profile (if running on EC2)

2. **Invalid/Expired Credentials**
   - IRSA role ARN is incorrect or doesn't exist
   - Static credentials have been rotated or revoked
   - Instance profile has been removed
   - Test credentials using: `kubectl exec <pod> -- aws sts get-caller-identity`

3. **IAM Permissions Missing**
   - The IAM role/user has credentials but lacks `secretsmanager:*` permissions
   - Check for SCPs or permission boundaries that might block access
   - Verify the role has the required Secrets Manager policy

4. **IRSA Misconfiguration** (EKS only)
   - Service account missing role ARN annotation
   - OIDC provider not configured correctly in EKS
   - Role trust policy doesn't allow the service account
   - Wrong namespace in trust policy condition

### Verify AWS Configuration

**Check if credentials are available (any method):**
```bash
POD=$(kubectl get pods -n db-system \
  -l control-plane=controller-manager \
  -o jsonpath='{.items[0].metadata.name}')

# Check IAM identity (works for any credential method)
kubectl exec -n db-system $POD -c manager -- \
  aws sts get-caller-identity
```

If this works, the pod has valid AWS credentials. The output shows which IAM principal is being used.

**Check environment variables (if using static credentials):**
```bash
kubectl exec -n db-system $POD -c manager -- \
  env | grep AWS
```

**Check IRSA annotation (if using IRSA):**
```bash
kubectl get serviceaccount database-user-operator \
  -n db-system \
  -o jsonpath='{.metadata.annotations.eks\.amazonaws\.com/role-arn}'
```

**Test Secrets Manager access:**
```bash
kubectl exec -n db-system $POD -c manager -- \
  aws secretsmanager list-secrets --region us-east-1 --max-items 1
```

### Where Are Credentials Stored?

**Created database credentials are ALWAYS stored in AWS Secrets Manager.**

The connection string source only determines where to read the ADMIN connection string:

- **Read admin connection from Kubernetes Secret:**
  ```yaml
  spec:
    connectionStringSecretRef:
      name: postgres-admin-connection
  ```
  - Admin credentials: Kubernetes Secret
  - Created credentials: AWS Secrets Manager ⚠️

- **Read admin connection from AWS Secrets Manager:**
  ```yaml
  spec:
    connectionStringAWSSecretRef:
      secretName: rds/admin/postgres-connection
      region: us-east-1
  ```
  - Admin credentials: AWS Secrets Manager
  - Created credentials: AWS Secrets Manager

Either way, **AWS permissions are always required** because created credentials always go to AWS Secrets Manager.

## Best Practices

1. **Use IRSA for EKS** - More secure than static credentials
2. **Rotate credentials regularly** - Set up automatic rotation in IAM
3. **Principle of least privilege** - Grant only required Secrets Manager permissions
4. **Use resource-level permissions** - Restrict access to specific secret paths
5. **Enable audit logging** - Use CloudTrail to monitor Secrets Manager access

## Example IAM Role Trust Policy (IRSA)

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::123456789012:oidc-provider/oidc.eks.us-east-1.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "oidc.eks.us-east-1.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE:sub": "system:serviceaccount:db-system:database-user-operator",
          "oidc.eks.us-east-1.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE:aud": "sts.amazonaws.com"
        }
      }
    }
  ]
}
```
