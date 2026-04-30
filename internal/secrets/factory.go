/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev1alpha1 "opzkit/database-user-operator/api/v1alpha1"
)

// ErrNoBackendConfigured is returned when a Database resource doesn't
// specify any of the supported destination backends.
var ErrNoBackendConfigured = errors.New("spec.secretBackend has no backend configured: set one of aws, kubernetes, infisical")

// ErrMultipleBackendsConfigured is returned when a Database resource
// specifies more than one destination backend simultaneously.
var ErrMultipleBackendsConfigured = errors.New("spec.secretBackend has multiple backends configured: set exactly one of aws, kubernetes, infisical")

// NewBackend selects and constructs a Backend implementation based on
// which spec.secretBackend.* field is populated. Returns
// ErrMultipleBackendsConfigured / ErrNoBackendConfigured if zero or
// more than one backend is set.
//
// k8sClient is used by the Kubernetes Secret backend to read/write
// Secrets in the cluster; the AWS backend ignores it. It is required
// to be non-nil when spec.secretBackend.kubernetes is set.
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

	// Phase 3:
	//   case sb.Infisical != nil:  return NewInfisicalBackend(...)

	default:
		return nil, ErrNoBackendConfigured
	}
}
