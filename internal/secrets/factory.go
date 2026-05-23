/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev1alpha1 "opzkit/database-user-operator/api/v1alpha1"
)

// ErrNoBackendConfigured is returned when a Database resource doesn't
// specify any of the supported destination backends.
var ErrNoBackendConfigured = errors.New("spec.secretBackend has no backend configured: set one of aws, kubernetes, infisical, scaleway")

// ErrMultipleBackendsConfigured is returned when a Database resource
// specifies more than one destination backend simultaneously.
var ErrMultipleBackendsConfigured = errors.New("spec.secretBackend has multiple backends configured: set exactly one of aws, kubernetes, infisical, scaleway")

// Standard data keys for the Infisical Universal Auth bootstrap Secret.
const (
	InfisicalAuthClientIDKey     = "clientId"
	InfisicalAuthClientSecretKey = "clientSecret"
)

// Standard data keys for the Scaleway IAM API-key bootstrap Secret.
// Mirrors the shape ESO's Scaleway provider expects, so the same Secret
// can back both stores when a cluster has ESO + DBUO.
const (
	ScalewayAuthAccessKeyKey = "access_key"
	ScalewayAuthSecretKeyKey = "secret_key"
)

// NewBackend selects and constructs a Backend implementation based on
// which spec.secretBackend.* field is populated. Returns
// ErrMultipleBackendsConfigured / ErrNoBackendConfigured if zero or
// more than one backend is set.
//
// k8sClient is used by the Kubernetes Secret backend (to read/write
// Secrets) and by the Infisical backend (to read the Universal Auth
// bootstrap Secret). It must be non-nil when either of those backends
// is selected; the AWS backend ignores it.
func NewBackend(ctx context.Context, db *databasev1alpha1.Database, k8sClient client.Client) (Backend, error) {
	sb := db.Spec.SecretBackend

	count := 0
	if sb.AWS != nil {
		count++
	}
	if sb.Kubernetes != nil {
		count++
	}
	if sb.Infisical != nil {
		count++
	}
	if sb.Scaleway != nil {
		count++
	}
	if count == 0 {
		return nil, ErrNoBackendConfigured
	}
	if count > 1 {
		return nil, ErrMultipleBackendsConfigured
	}

	switch {
	case sb.AWS != nil:
		if err := ValidateRegion(sb.AWS.Region); err != nil {
			return nil, err
		}
		return NewAWSSecretsManagerClient(ctx, sb.AWS.Region)

	case sb.Kubernetes != nil:
		if k8sClient == nil {
			return nil, errors.New("kubernetes secret backend requires a non-nil k8s client")
		}
		namespace := sb.Kubernetes.Namespace
		if namespace == "" {
			namespace = db.Namespace
		}
		return NewKubernetesBackend(k8sClient, namespace), nil

	case sb.Infisical != nil:
		if k8sClient == nil {
			return nil, errors.New("infisical secret backend requires a non-nil k8s client (to read the universal-auth bootstrap Secret)")
		}
		auth, err := readInfisicalAuth(ctx, k8sClient, db.Namespace, sb.Infisical.AuthSecretRef.Name)
		if err != nil {
			return nil, err
		}
		return NewInfisicalBackend(
			sb.Infisical.HostAPI,
			sb.Infisical.ProjectID,
			sb.Infisical.Environment,
			sb.Infisical.SecretsPath,
			auth,
		), nil

	case sb.Scaleway != nil:
		var refName string
		if sb.Scaleway.AuthSecretRef != nil {
			refName = sb.Scaleway.AuthSecretRef.Name
		}
		auth, err := LoadScalewayAuth(ctx, k8sClient, db.Namespace, refName)
		if err != nil {
			return nil, err
		}
		return NewScalewayBackend(
			sb.Scaleway.Region,
			sb.Scaleway.ProjectID,
			auth,
		)

	default:
		return nil, ErrNoBackendConfigured
	}
}

// readInfisicalAuth reads the clientId/clientSecret pair from the
// Kubernetes Secret referenced by spec.secretBackend.infisical.authSecretRef.
// The Secret must live in the same namespace as the Database resource.
func readInfisicalAuth(ctx context.Context, k8sClient client.Client, namespace, name string) (InfisicalAuth, error) {
	var s corev1.Secret
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &s); err != nil {
		return InfisicalAuth{}, fmt.Errorf("failed to read infisical auth secret %s/%s: %w", namespace, name, err)
	}
	clientID := string(s.Data[InfisicalAuthClientIDKey])
	clientSecret := string(s.Data[InfisicalAuthClientSecretKey])
	if clientID == "" || clientSecret == "" {
		return InfisicalAuth{}, fmt.Errorf("infisical auth secret %s/%s missing %q or %q key", namespace, name, InfisicalAuthClientIDKey, InfisicalAuthClientSecretKey)
	}
	return InfisicalAuth{ClientID: clientID, ClientSecret: clientSecret}, nil
}

// Operator-pod environment variables read by LoadScalewayAuth when
// `spec.{secretBackend,connectionString}.scaleway.authSecretRef` is
// omitted on the Database CR. Matches the variable names the Scaleway
// SDK loader (`scw.WithEnv()`) expects, so the same env can also back
// other client constructions inside the pod without translation.
const (
	ScalewayEnvAccessKey = "SCW_ACCESS_KEY"
	ScalewayEnvSecretKey = "SCW_SECRET_KEY" //nolint:gosec // env var name, not a credential
)

// LoadScalewayAuth returns Scaleway IAM credentials sourced from one
// of two places:
//
//  1. If `secretName` is non-empty, read access_key + secret_key from
//     a Kubernetes Secret named `secretName` in `namespace` (the
//     Database's namespace).
//  2. Otherwise fall back to the operator pod's `SCW_ACCESS_KEY` +
//     `SCW_SECRET_KEY` environment variables — mirrors the AWS-backend
//     IRSA / instance-profile pattern, removing the need for a
//     per-namespace credential Secret.
//
// Returns a descriptive error if neither source yields a complete
// (access_key, secret_key) pair. The k8s-Secret path requires
// `k8sClient` to be non-nil; the env path does not.
//
// Security trade-off: the env path uses ONE operator-pod IAM key for
// every Database CR cluster-wide. Per-namespace RBAC no longer scopes
// which Scaleway Project a CR can touch — `spec.scaleway.projectID`
// on any CR is reachable as long as the operator key has IAM on that
// Project. Scope the operator key to the Project set all consuming
// namespaces are allowed to touch, or stay on the per-CR
// `authSecretRef` path when tenant isolation matters.
func LoadScalewayAuth(ctx context.Context, k8sClient client.Client, namespace, secretName string) (ScalewayAuth, error) {
	if secretName != "" {
		if k8sClient == nil {
			return ScalewayAuth{}, errors.New("scaleway auth requires a non-nil k8s client to read authSecretRef")
		}
		return readScalewayAuth(ctx, k8sClient, namespace, secretName)
	}
	accessKey := os.Getenv(ScalewayEnvAccessKey)
	secretKey := os.Getenv(ScalewayEnvSecretKey)
	if accessKey == "" || secretKey == "" {
		return ScalewayAuth{}, fmt.Errorf("scaleway auth: spec.{secretBackend,connectionString}.scaleway.authSecretRef is omitted and operator env vars %s + %s are not both set", ScalewayEnvAccessKey, ScalewayEnvSecretKey)
	}
	return ScalewayAuth{AccessKey: accessKey, SecretKey: secretKey}, nil
}

// readScalewayAuth reads the access_key/secret_key pair from the
// Kubernetes Secret referenced by spec.secretBackend.scaleway.authSecretRef.
// The Secret must live in the same namespace as the Database resource.
func readScalewayAuth(ctx context.Context, k8sClient client.Client, namespace, name string) (ScalewayAuth, error) {
	var s corev1.Secret
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &s); err != nil {
		return ScalewayAuth{}, fmt.Errorf("failed to read scaleway auth secret %s/%s: %w", namespace, name, err)
	}
	accessKey := string(s.Data[ScalewayAuthAccessKeyKey])
	secretKey := string(s.Data[ScalewayAuthSecretKeyKey])
	if accessKey == "" || secretKey == "" {
		return ScalewayAuth{}, fmt.Errorf("scaleway auth secret %s/%s missing %q or %q key", namespace, name, ScalewayAuthAccessKeyKey, ScalewayAuthSecretKeyKey)
	}
	return ScalewayAuth{AccessKey: accessKey, SecretKey: secretKey}, nil
}
