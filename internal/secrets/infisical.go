/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	infisical "github.com/infisical/go-sdk"
)

// InfisicalSecretClient is the subset of the Infisical SDK's secret-management
// surface that the InfisicalBackend uses. Defining it as an interface here
// lets tests swap in an in-memory fake.
type InfisicalSecretClient interface {
	Create(opts infisical.CreateSecretOptions) (any, error)
	Retrieve(opts infisical.RetrieveSecretOptions) (string, error)
	Update(opts infisical.UpdateSecretOptions) (any, error)
	Delete(opts infisical.DeleteSecretOptions) (any, error)
}

// infisicalSDKClient adapts the real SDK client to InfisicalSecretClient.
// The SDK exposes methods that return concrete model types; our interface
// hides those so tests don't need to construct them.
type infisicalSDKClient struct {
	inner infisical.SecretsInterface
}

func (c *infisicalSDKClient) Create(opts infisical.CreateSecretOptions) (any, error) {
	return c.inner.Create(opts)
}

func (c *infisicalSDKClient) Retrieve(opts infisical.RetrieveSecretOptions) (string, error) {
	s, err := c.inner.Retrieve(opts)
	if err != nil {
		return "", err
	}
	return s.SecretValue, nil
}

func (c *infisicalSDKClient) Update(opts infisical.UpdateSecretOptions) (any, error) {
	return c.inner.Update(opts)
}

func (c *infisicalSDKClient) Delete(opts infisical.DeleteSecretOptions) (any, error) {
	return c.inner.Delete(opts)
}

// InfisicalAuth represents the Universal Auth credential pair used to
// authenticate against Infisical. The pair is sourced from a Kubernetes
// Secret in the cluster; reading it is the controller's responsibility,
// not this package's.
type InfisicalAuth struct {
	ClientID     string
	ClientSecret string
}

// InfisicalBackend stores generated database credentials in Infisical
// (Cloud or self-hosted) via Universal Auth. The DatabaseSecret JSON
// blob is stored as the value of a single secret keyed by a sanitised
// version of the supplied name (slashes replaced with underscores so
// the result is a valid Infisical secret key).
//
// Infisical's V3 secret API does not expose per-secret version IDs, so
// the version returned from Create/Update is always "".
//
// SyncTags is a no-op: Infisical tags are first-class entities with
// their own UUIDs and lifecycle, mapping AWS-style key/value tags onto
// them is non-trivial and out of scope for now.
type InfisicalBackend struct {
	client      InfisicalSecretClient
	projectID   string
	environment string
	secretsPath string

	auth      InfisicalAuth
	loginer   func(InfisicalAuth) error
	loginOnce sync.Once
	loginErr  error
}

// NewInfisicalBackend constructs an InfisicalBackend backed by the real
// Infisical SDK. siteURL is typically "https://app.infisical.com" for
// Cloud, or your self-hosted URL.
func NewInfisicalBackend(siteURL, projectID, environment, secretsPath string, auth InfisicalAuth) *InfisicalBackend {
	if siteURL == "" {
		siteURL = "https://app.infisical.com"
	}
	if secretsPath == "" {
		secretsPath = "/"
	}

	sdk := infisical.NewInfisicalClient(context.Background(), infisical.Config{
		SiteUrl:          siteURL,
		AutoTokenRefresh: true,
	})

	return &InfisicalBackend{
		client: &infisicalSDKClient{inner: sdk.Secrets()},
		loginer: func(a InfisicalAuth) error {
			_, err := sdk.Auth().UniversalAuthLogin(a.ClientID, a.ClientSecret)
			return err
		},
		projectID:   projectID,
		environment: environment,
		secretsPath: secretsPath,
		auth:        auth,
	}
}

// newInfisicalBackendWithClient is used by tests to inject a fake client.
func newInfisicalBackendWithClient(client InfisicalSecretClient, projectID, environment, secretsPath string, loginer func(InfisicalAuth) error, auth InfisicalAuth) *InfisicalBackend {
	if secretsPath == "" {
		secretsPath = "/"
	}
	return &InfisicalBackend{
		client:      client,
		loginer:     loginer,
		projectID:   projectID,
		environment: environment,
		secretsPath: secretsPath,
		auth:        auth,
	}
}

// Compile-time check that *InfisicalBackend satisfies Backend.
var _ Backend = (*InfisicalBackend)(nil)

func (b *InfisicalBackend) login() error {
	b.loginOnce.Do(func() {
		if b.loginer == nil {
			return
		}
		if err := b.loginer(b.auth); err != nil {
			b.loginErr = fmt.Errorf("infisical universal auth login failed: %w", err)
		}
	})
	return b.loginErr
}

// key normalises an arbitrary "secret name" (which may include slashes
// from the AWS-style rds/<engine>/<db> default) into a valid Infisical
// secret key.
func (b *InfisicalBackend) key(name string) string {
	return strings.ReplaceAll(strings.Trim(name, "/"), "/", "_")
}

// locator returns a stable backend-specific identifier suitable for
// status reporting.
func (b *InfisicalBackend) locator(name string) string {
	path := b.secretsPath
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return fmt.Sprintf("infisical://%s/%s%s%s", b.projectID, b.environment, path, b.key(name))
}

// Exists implements Backend.
func (b *InfisicalBackend) Exists(ctx context.Context, name string) (bool, error) {
	if err := b.login(); err != nil {
		return false, err
	}
	_, err := b.client.Retrieve(infisical.RetrieveSecretOptions{
		ProjectID:   b.projectID,
		Environment: b.environment,
		SecretKey:   b.key(name),
		SecretPath:  b.secretsPath,
	})
	if err != nil {
		if isInfisicalNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("infisical retrieve failed: %w", err)
	}
	return true, nil
}

// Get implements Backend.
func (b *InfisicalBackend) Get(ctx context.Context, name string) (*DatabaseSecret, error) {
	if err := b.login(); err != nil {
		return nil, err
	}
	value, err := b.client.Retrieve(infisical.RetrieveSecretOptions{
		ProjectID:   b.projectID,
		Environment: b.environment,
		SecretKey:   b.key(name),
		SecretPath:  b.secretsPath,
	})
	if err != nil {
		if isInfisicalNotFound(err) {
			return nil, &SecretNotFoundError{SecretName: b.locator(name), Err: err}
		}
		return nil, fmt.Errorf("infisical retrieve failed: %w", err)
	}
	var dbSecret DatabaseSecret
	if err := json.Unmarshal([]byte(value), &dbSecret); err != nil {
		return nil, fmt.Errorf("failed to unmarshal infisical secret value at %s: %w", b.locator(name), err)
	}
	return &dbSecret, nil
}

// Create implements Backend. If the secret already exists, its value is
// overwritten via Update — same restore-on-create semantics as AWS / K8s.
func (b *InfisicalBackend) Create(ctx context.Context, name, description string, secret *DatabaseSecret, tags map[string]string, template string) (string, string, error) {
	if err := b.login(); err != nil {
		return "", "", err
	}
	payload, err := secret.ToJSONWithTemplate(template)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal database secret: %w", err)
	}

	_, err = b.client.Create(infisical.CreateSecretOptions{
		ProjectID:     b.projectID,
		Environment:   b.environment,
		SecretKey:     b.key(name),
		SecretValue:   string(payload),
		SecretPath:    b.secretsPath,
		SecretComment: description,
	})
	if err != nil {
		if !isInfisicalAlreadyExists(err) {
			return "", "", fmt.Errorf("infisical create failed: %w", err)
		}
		// Already exists — overwrite via Update.
		if _, uerr := b.client.Update(infisical.UpdateSecretOptions{
			ProjectID:      b.projectID,
			Environment:    b.environment,
			SecretKey:      b.key(name),
			NewSecretValue: string(payload),
			SecretPath:     b.secretsPath,
		}); uerr != nil {
			return "", "", fmt.Errorf("infisical create-then-update failed: %w", uerr)
		}
	}

	return b.locator(name), "", nil
}

// Update implements Backend.
func (b *InfisicalBackend) Update(ctx context.Context, name string, secret *DatabaseSecret, template string) (string, error) {
	if err := b.login(); err != nil {
		return "", err
	}
	payload, err := secret.ToJSONWithTemplate(template)
	if err != nil {
		return "", fmt.Errorf("failed to marshal database secret: %w", err)
	}

	_, err = b.client.Update(infisical.UpdateSecretOptions{
		ProjectID:      b.projectID,
		Environment:    b.environment,
		SecretKey:      b.key(name),
		NewSecretValue: string(payload),
		SecretPath:     b.secretsPath,
	})
	if err != nil {
		if isInfisicalNotFound(err) {
			return "", &SecretNotFoundError{SecretName: b.locator(name), Err: err}
		}
		return "", fmt.Errorf("infisical update failed: %w", err)
	}
	return "", nil
}

// Delete implements Backend. forceDelete is ignored — Infisical doesn't
// have a soft-delete state.
func (b *InfisicalBackend) Delete(ctx context.Context, name string, forceDelete bool) error {
	if err := b.login(); err != nil {
		return err
	}
	_, err := b.client.Delete(infisical.DeleteSecretOptions{
		ProjectID:   b.projectID,
		Environment: b.environment,
		SecretKey:   b.key(name),
		SecretPath:  b.secretsPath,
	})
	if err != nil {
		if isInfisicalNotFound(err) {
			return nil
		}
		return fmt.Errorf("infisical delete failed: %w", err)
	}
	return nil
}

// Locator implements Backend.
func (b *InfisicalBackend) Locator(ctx context.Context, name string) (string, error) {
	return b.locator(name), nil
}

// SyncTags implements Backend as a no-op. Infisical tags are first-class
// entities with their own UUIDs; mapping them from AWS-style key/value
// tags is out of scope.
func (b *InfisicalBackend) SyncTags(ctx context.Context, name string, desired map[string]string) error {
	return nil
}

// isInfisicalNotFound returns true if the SDK error indicates the secret
// doesn't exist. The SDK doesn't currently expose typed errors for this
// (as of v0.7.x), so we fall back to a substring match.
func isInfisicalNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "secretnotfound")
}

// isInfisicalAlreadyExists returns true if the SDK error indicates the
// secret already exists.
func isInfisicalAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "secretalreadyexists")
}
