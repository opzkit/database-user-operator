/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import (
	"context"
	"fmt"
)

// This file adapts the existing AWSSecretsManagerClient to the generic
// Backend interface. Existing methods (SecretExists, GetSecretARN,
// CreateSecretWithTemplate, …) are kept for use sites that still need
// AWS-specific behaviour (e.g. cross-region migration); the methods
// below provide a backend-agnostic surface for the controller to use
// going forward.

// Compile-time check that AWSSecretsManagerClient satisfies Backend.
var _ Backend = (*AWSSecretsManagerClient)(nil)

// Exists implements Backend.
func (c *AWSSecretsManagerClient) Exists(ctx context.Context, name string) (bool, error) {
	return c.SecretExists(ctx, name)
}

// Get implements Backend.
func (c *AWSSecretsManagerClient) Get(ctx context.Context, name string) (*DatabaseSecret, error) {
	return c.GetSecret(ctx, name)
}

// Create implements Backend.
func (c *AWSSecretsManagerClient) Create(ctx context.Context, name, description string, secret *DatabaseSecret, tags map[string]string, template string) (string, string, error) {
	return c.CreateSecretWithTemplate(ctx, name, description, secret, tags, template)
}

// Update implements Backend.
func (c *AWSSecretsManagerClient) Update(ctx context.Context, name string, secret *DatabaseSecret, template string) (string, error) {
	return c.UpdateSecretWithTemplate(ctx, name, secret, template)
}

// Delete implements Backend.
func (c *AWSSecretsManagerClient) Delete(ctx context.Context, name string, forceDelete bool) error {
	return c.DeleteSecret(ctx, name, forceDelete)
}

// Locator implements Backend.
func (c *AWSSecretsManagerClient) Locator(ctx context.Context, name string) (string, error) {
	return c.GetSecretARN(ctx, name)
}

// SyncTags implements Backend by computing the diff between the secret's
// current tags and the desired set, then issuing untag + tag calls.
func (c *AWSSecretsManagerClient) SyncTags(ctx context.Context, name string, desired map[string]string) error {
	existing, err := c.GetSecretTags(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get existing secret tags: %w", err)
	}

	var toRemove []string
	for k := range existing {
		if _, keep := desired[k]; !keep {
			toRemove = append(toRemove, k)
		}
	}

	if len(toRemove) > 0 {
		if err := c.UntagSecret(ctx, name, toRemove); err != nil {
			return fmt.Errorf("failed to remove secret tags: %w", err)
		}
	}

	if len(desired) > 0 {
		if err := c.TagSecret(ctx, name, desired); err != nil {
			return fmt.Errorf("failed to apply secret tags: %w", err)
		}
	}

	return nil
}
