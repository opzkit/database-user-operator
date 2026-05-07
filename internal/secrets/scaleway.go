/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	smapi "github.com/scaleway/scaleway-sdk-go/api/secret/v1beta1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ScalewayAuth represents the Scaleway IAM API key pair (access_key +
// secret_key) used to authenticate against Secret Manager. Sourced from a
// Kubernetes Secret in the cluster; reading it is the controller's
// responsibility, not this package's.
type ScalewayAuth struct {
	AccessKey string
	SecretKey string
}

// ScalewaySMClient is the subset of the Scaleway Secret Manager SDK
// surface that ScalewayBackend uses. Defining it as an interface lets
// tests swap in an in-memory fake.
type ScalewaySMClient interface {
	ListSecrets(req *smapi.ListSecretsRequest, opts ...scw.RequestOption) (*smapi.ListSecretsResponse, error)
	CreateSecret(req *smapi.CreateSecretRequest, opts ...scw.RequestOption) (*smapi.Secret, error)
	UpdateSecret(req *smapi.UpdateSecretRequest, opts ...scw.RequestOption) (*smapi.Secret, error)
	DeleteSecret(req *smapi.DeleteSecretRequest, opts ...scw.RequestOption) error
	CreateSecretVersion(req *smapi.CreateSecretVersionRequest, opts ...scw.RequestOption) (*smapi.SecretVersion, error)
	AccessSecretVersion(req *smapi.AccessSecretVersionRequest, opts ...scw.RequestOption) (*smapi.AccessSecretVersionResponse, error)
}

// ScalewayBackend stores generated database credentials in Scaleway
// Secret Manager. The DatabaseSecret JSON blob is stored verbatim as
// the version payload on a Scaleway Secret named after a sanitised
// version of the supplied name (slashes replaced with underscores so
// the result is a valid Scaleway Secret name).
//
// Each Update creates a new SecretVersion; the returned version string
// is the SecretVersion Revision (1-based monotonic).
type ScalewayBackend struct {
	client    ScalewaySMClient
	region    scw.Region
	projectID string
}

// NewScalewayBackend constructs a ScalewayBackend authenticated with
// the supplied IAM API key against the given region + project.
func NewScalewayBackend(region, projectID string, auth ScalewayAuth) (*ScalewayBackend, error) {
	r, err := parseScalewayRegion(region)
	if err != nil {
		return nil, err
	}
	if projectID == "" {
		return nil, errors.New("scaleway secret backend: projectID is required")
	}
	if auth.AccessKey == "" || auth.SecretKey == "" {
		return nil, errors.New("scaleway secret backend: auth.AccessKey and auth.SecretKey are required")
	}

	scwClient, err := scw.NewClient(
		scw.WithAuth(auth.AccessKey, auth.SecretKey),
		scw.WithDefaultRegion(r),
		scw.WithDefaultProjectID(projectID),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to construct scaleway client: %w", err)
	}

	return &ScalewayBackend{
		client:    smapi.NewAPI(scwClient),
		region:    r,
		projectID: projectID,
	}, nil
}

// newScalewayBackendWithClient is used by tests to inject a fake client.
func newScalewayBackendWithClient(client ScalewaySMClient, region scw.Region, projectID string) *ScalewayBackend {
	return &ScalewayBackend{client: client, region: region, projectID: projectID}
}

// Compile-time check that *ScalewayBackend satisfies Backend.
var _ Backend = (*ScalewayBackend)(nil)

// parseScalewayRegion validates and converts a region string. Empty is
// rejected — the controller path always provides one (CRD field is
// required + enum-validated).
func parseScalewayRegion(region string) (scw.Region, error) {
	if region == "" {
		return "", errors.New("scaleway secret backend: region is required")
	}
	r, err := scw.ParseRegion(region)
	if err != nil {
		return "", fmt.Errorf("invalid scaleway region %q: %w", region, err)
	}
	return r, nil
}

// scalewayName normalises an arbitrary "secret name" (which may include
// slashes from the AWS-style rds/<engine>/<db> default) into a valid
// Scaleway Secret name. Scaleway accepts [A-Za-z0-9-_.] in names.
func scalewayName(name string) string {
	n := strings.Trim(name, "/")
	n = strings.ReplaceAll(n, "/", "_")
	return n
}

// locator returns a stable backend-specific identifier for the secret.
// Uses (region, projectID, sanitised-name) so it stays human-readable
// and is independent of the secret's UUID at lookup time.
func (b *ScalewayBackend) locator(name string) string {
	return fmt.Sprintf("scaleway://%s/%s/%s", b.region, b.projectID, scalewayName(name))
}

// findSecret looks up a Scaleway Secret by name+project. Returns
// (*smapi.Secret, true, nil) on hit, (nil, false, nil) on miss.
func (b *ScalewayBackend) findSecret(ctx context.Context, name string) (*smapi.Secret, bool, error) {
	n := scalewayName(name)
	resp, err := b.client.ListSecrets(&smapi.ListSecretsRequest{
		Region:    b.region,
		ProjectID: &b.projectID,
		Name:      &n,
	}, scw.WithContext(ctx))
	if err != nil {
		return nil, false, fmt.Errorf("scaleway list-secrets failed: %w", err)
	}
	for _, s := range resp.Secrets {
		if s.Name == n {
			return s, true, nil
		}
	}
	return nil, false, nil
}

// Exists implements Backend.
func (b *ScalewayBackend) Exists(ctx context.Context, name string) (bool, error) {
	_, ok, err := b.findSecret(ctx, name)
	return ok, err
}

// Get implements Backend.
func (b *ScalewayBackend) Get(ctx context.Context, name string) (*DatabaseSecret, error) {
	s, ok, err := b.findSecret(ctx, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &SecretNotFoundError{SecretName: b.locator(name)}
	}
	access, err := b.client.AccessSecretVersion(&smapi.AccessSecretVersionRequest{
		Region:   b.region,
		SecretID: s.ID,
		Revision: "latest_enabled",
	}, scw.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("scaleway access-secret-version failed: %w", err)
	}
	var dbSecret DatabaseSecret
	if err := json.Unmarshal(access.Data, &dbSecret); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scaleway secret payload at %s: %w", b.locator(name), err)
	}
	return &dbSecret, nil
}

// Create implements Backend. If a Secret with this name already exists
// (including soft-deleted state — Scaleway has no soft-delete window for
// SM, so this just means the resource is present), Create overwrites by
// adding a new version, matching the AWS / Kubernetes / Infisical
// "restore-on-create" behaviour the controller relies on.
func (b *ScalewayBackend) Create(ctx context.Context, name, description string, secret *DatabaseSecret, tags map[string]string, template string) (string, string, error) {
	payload, err := secret.ToJSONWithTemplate(template)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal database secret: %w", err)
	}

	existing, ok, err := b.findSecret(ctx, name)
	if err != nil {
		return "", "", err
	}

	var secretID string
	if ok {
		secretID = existing.ID
		// Update tags + description in case they drifted.
		desiredTags := tagsToScalewaySlice(tags)
		updateReq := &smapi.UpdateSecretRequest{
			Region:   b.region,
			SecretID: secretID,
			Tags:     &desiredTags,
		}
		if description != "" {
			d := description
			updateReq.Description = &d
		}
		if _, err := b.client.UpdateSecret(updateReq, scw.WithContext(ctx)); err != nil {
			return "", "", fmt.Errorf("scaleway update-secret (description/tags) failed: %w", err)
		}
	} else {
		createReq := &smapi.CreateSecretRequest{
			Region:    b.region,
			ProjectID: b.projectID,
			Name:      scalewayName(name),
			Tags:      tagsToScalewaySlice(tags),
		}
		if description != "" {
			d := description
			createReq.Description = &d
		}
		created, err := b.client.CreateSecret(createReq, scw.WithContext(ctx))
		if err != nil {
			return "", "", fmt.Errorf("scaleway create-secret failed: %w", err)
		}
		secretID = created.ID
	}

	version, err := b.client.CreateSecretVersion(&smapi.CreateSecretVersionRequest{
		Region:   b.region,
		SecretID: secretID,
		Data:     payload,
	}, scw.WithContext(ctx))
	if err != nil {
		return "", "", fmt.Errorf("scaleway create-secret-version failed: %w", err)
	}
	return b.locator(name), fmt.Sprintf("%d", version.Revision), nil
}

// Update implements Backend.
func (b *ScalewayBackend) Update(ctx context.Context, name string, secret *DatabaseSecret, template string) (string, error) {
	s, ok, err := b.findSecret(ctx, name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", &SecretNotFoundError{SecretName: b.locator(name)}
	}
	payload, err := secret.ToJSONWithTemplate(template)
	if err != nil {
		return "", fmt.Errorf("failed to marshal database secret: %w", err)
	}
	version, err := b.client.CreateSecretVersion(&smapi.CreateSecretVersionRequest{
		Region:   b.region,
		SecretID: s.ID,
		Data:     payload,
	}, scw.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("scaleway create-secret-version failed: %w", err)
	}
	return fmt.Sprintf("%d", version.Revision), nil
}

// Delete implements Backend. Scaleway Secret Manager has no soft-delete
// recovery window, so forceDelete is ignored. Deleting a non-existent
// secret is not an error.
func (b *ScalewayBackend) Delete(ctx context.Context, name string, forceDelete bool) error {
	s, ok, err := b.findSecret(ctx, name)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := b.client.DeleteSecret(&smapi.DeleteSecretRequest{
		Region:   b.region,
		SecretID: s.ID,
	}, scw.WithContext(ctx)); err != nil {
		return fmt.Errorf("scaleway delete-secret failed: %w", err)
	}
	return nil
}

// Locator implements Backend.
func (b *ScalewayBackend) Locator(ctx context.Context, name string) (string, error) {
	return b.locator(name), nil
}

// SyncTags implements Backend by replacing the secret's tag set with
// the desired one. Scaleway's UpdateSecret accepts the full tag list,
// so this is a single API call.
func (b *ScalewayBackend) SyncTags(ctx context.Context, name string, desired map[string]string) error {
	s, ok, err := b.findSecret(ctx, name)
	if err != nil {
		return err
	}
	if !ok {
		return &SecretNotFoundError{SecretName: b.locator(name)}
	}
	tags := tagsToScalewaySlice(desired)
	if _, err := b.client.UpdateSecret(&smapi.UpdateSecretRequest{
		Region:   b.region,
		SecretID: s.ID,
		Tags:     &tags,
	}, scw.WithContext(ctx)); err != nil {
		return fmt.Errorf("scaleway update-secret (tags) failed: %w", err)
	}
	return nil
}

// tagsToScalewaySlice flattens a map into Scaleway's []string tag
// shape. Each entry serialises as "key=value"; an empty map yields an
// empty (non-nil) slice so UpdateSecret clears tags rather than
// leaving the existing set untouched.
func tagsToScalewaySlice(tags map[string]string) []string {
	out := make([]string, 0, len(tags))
	for k, v := range tags {
		if v == "" {
			out = append(out, k)
		} else {
			out = append(out, fmt.Sprintf("%s=%s", k, v))
		}
	}
	sort.Strings(out)
	return out
}
