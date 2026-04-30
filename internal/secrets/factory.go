/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import (
	"context"
	"errors"

	databasev1alpha1 "opzkit/database-user-operator/api/v1alpha1"
)

// ErrNoBackendConfigured is returned when a Database resource doesn't
// specify any of the supported destination backends.
var ErrNoBackendConfigured = errors.New("no destination backend configured: set spec.awsSecretsManager (later: spec.kubernetesSecret, spec.infisical)")

// NewBackend selects and constructs a Backend implementation based on
// which spec field is populated on the Database CR. Region resolution and
// other backend-specific defaulting happens in the implementation
// constructors.
func NewBackend(ctx context.Context, db *databasev1alpha1.Database) (Backend, error) {
	switch {
	case db.Spec.AWSSecretsManager != nil:
		region := db.Spec.AWSSecretsManager.Region
		if err := ValidateRegion(region); err != nil {
			return nil, err
		}
		return NewAWSSecretsManagerClient(ctx, region)

	// Future:
	//   case db.Spec.KubernetesSecret != nil: return NewKubernetesBackend(...)
	//   case db.Spec.Infisical != nil:        return NewInfisicalBackend(...)

	default:
		return nil, ErrNoBackendConfigured
	}
}
