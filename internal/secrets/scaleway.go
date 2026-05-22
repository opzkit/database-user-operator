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
// the version payload on a Scaleway Secret.
//
// The supplied `spec.secretName` is split on the last `/` into
// (Path, Name): leading segments become the Scaleway Secret Path
// (folder), the trailing segment is the Secret Name. This matches
// the AWS Secrets Manager backend, where slashes stay in the literal
// name — both backends therefore land at the same logical location
// (default `rds/postgres/<dbName>`).
//
// For backwards compatibility with secrets previously written by
// pre-path versions of this operator (slashes flattened to `_`),
// findSecret falls back to a legacy sanitised-name lookup at root
// path when the path-style lookup misses. New writes always use the
// path-aware shape.
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

// scalewayPathAndName splits an arbitrary "secret name" (which may
// include slashes from the AWS-style rds/<engine>/<db> default) into
// the (Path, Name) pair Scaleway Secret Manager uses. The last
// `/`-segment is the Name; everything before it (leading + trailing
// `/` enforced) is the Path. Single-segment input lands at root
// path `/`. Scaleway accepts [A-Za-z0-9-_.] in names and arbitrary
// `/`-separated folder paths.
//
//	"rds/postgres/foo"   -> ("/rds/postgres", "foo")
//	"/rds/postgres/foo"  -> ("/rds/postgres", "foo")
//	"/rds/postgres/foo/" -> ("/rds/postgres", "foo")
//	"foo"                -> ("/",             "foo")
//	""                   -> ("/",             "")
func scalewayPathAndName(input string) (string, string) {
	trimmed := strings.Trim(input, "/")
	if trimmed == "" {
		return "/", ""
	}
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		return "/", trimmed
	}
	return "/" + trimmed[:idx], trimmed[idx+1:]
}

// scalewayLegacyName reproduces the pre-path sanitisation (slashes
// flattened to underscores) used by older releases of this operator.
// Retained for read-side fallback so existing Secrets remain findable
// after upgrade.
func scalewayLegacyName(name string) string {
	n := strings.Trim(name, "/")
	return strings.ReplaceAll(n, "/", "_")
}

// locator returns a stable backend-specific identifier for the secret.
// Uses (region, projectID, path+name) so it stays human-readable and is
// independent of the secret's UUID at lookup time.
func (b *ScalewayBackend) locator(name string) string {
	path, n := scalewayPathAndName(name)
	sep := "/"
	if path == "/" {
		// Root path already supplies the separator — avoid `//<name>`.
		sep = ""
	}
	return fmt.Sprintf("scaleway://%s/%s%s%s%s", b.region, b.projectID, path, sep, n)
}

// findSecret looks up a Scaleway Secret by (Path, Name, project).
// On a miss, falls back to a legacy sanitised-name lookup at root
// path so secrets written by pre-path operator versions remain
// readable. Returns (*smapi.Secret, true, nil) on hit,
// (nil, false, nil) on miss.
func (b *ScalewayBackend) findSecret(ctx context.Context, name string) (*smapi.Secret, bool, error) {
	path, n := scalewayPathAndName(name)
	resp, err := b.client.ListSecrets(&smapi.ListSecretsRequest{
		Region:    b.region,
		ProjectID: &b.projectID,
		Name:      &n,
		Path:      &path,
	}, scw.WithContext(ctx))
	if err != nil {
		return nil, false, fmt.Errorf("scaleway list-secrets failed: %w", err)
	}
	for _, s := range resp.Secrets {
		if s.Name == n && s.Path == path {
			return s, true, nil
		}
	}

	// Legacy fallback: pre-path releases wrote secrets at root path
	// with `/` flattened to `_`. Only consult when the path-style
	// lookup truly missed and the legacy form differs from the
	// path-style name (i.e. the input contained at least one `/`).
	legacy := scalewayLegacyName(name)
	if legacy == n {
		return nil, false, nil
	}
	rootPath := "/"
	resp, err = b.client.ListSecrets(&smapi.ListSecretsRequest{
		Region:    b.region,
		ProjectID: &b.projectID,
		Name:      &legacy,
		Path:      &rootPath,
	}, scw.WithContext(ctx))
	if err != nil {
		return nil, false, fmt.Errorf("scaleway list-secrets (legacy) failed: %w", err)
	}
	for _, s := range resp.Secrets {
		if s.Name == legacy && (s.Path == rootPath || s.Path == "") {
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

// GetRawAt fetches the raw payload of the latest enabled version of
// the Scaleway Secret addressed by (path, name). Unlike Get, the
// payload is returned verbatim — no DatabaseSecret unmarshal — so
// callers reading admin-DSN secrets that were not written by this
// operator can parse arbitrary shapes.
//
// Returns SecretNotFoundError when the secret does not exist; the
// returned locator uses the supplied (path, name) directly rather
// than the path-split heuristic so error messages match what the
// caller asked for.
func (b *ScalewayBackend) GetRawAt(ctx context.Context, path, name string) ([]byte, error) {
	resp, err := b.client.ListSecrets(&smapi.ListSecretsRequest{
		Region:    b.region,
		ProjectID: &b.projectID,
		Name:      &name,
		Path:      &path,
	}, scw.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("scaleway list-secrets failed: %w", err)
	}
	var secret *smapi.Secret
	for _, s := range resp.Secrets {
		if s.Name == name && s.Path == path {
			secret = s
			break
		}
	}
	if secret == nil {
		return nil, &SecretNotFoundError{SecretName: fmt.Sprintf("scaleway://%s/%s%s/%s", b.region, b.projectID, strings.TrimRight(path, "/"), name)}
	}
	access, err := b.client.AccessSecretVersion(&smapi.AccessSecretVersionRequest{
		Region:   b.region,
		SecretID: secret.ID,
		Revision: "latest_enabled",
	}, scw.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("scaleway access-secret-version failed: %w", err)
	}
	return access.Data, nil
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
		path, n := scalewayPathAndName(name)
		createReq := &smapi.CreateSecretRequest{
			Region:    b.region,
			ProjectID: b.projectID,
			Name:      n,
			Path:      &path,
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
