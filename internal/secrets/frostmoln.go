/*
Copyright 2026 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Operator-pod environment for the Frostmoln connection-string source.
const (
	FrostmolnEnvTenantID  = "FROSTMOLN_TENANT_ID"
	FrostmolnEnvTokenFile = "FROSTMOLN_TOKEN_FILE" //nolint:gosec // env var name, not a credential
	FrostmolnEnvAPIURL    = "FROSTMOLN_API_URL"

	FrostmolnDefaultAPIURL    = "https://api.frostmoln.cloud"
	FrostmolnDefaultTokenFile = "/var/run/secrets/frostmoln/token" //nolint:gosec // mount path, not a credential
)

// frostmolnMaxResponseBytes caps how much of a lookup response is read.
// Secret values are small; anything larger is not a lookup response.
const frostmolnMaxResponseBytes = 1 << 20

// FrostmolnClient reads secret values through the Frostmoln secrets
// lookup API, authenticated with a workload identity token.
//
// The token is read from tokenFile on every request rather than once at
// construction: the workload-identity add-on rotates the short-lived
// token in its Secret, and kubelet refreshes the mounted file.
type FrostmolnClient struct {
	baseURL    string
	tenantID   string
	tokenFile  string
	httpClient *http.Client
}

// NewFrostmolnClient constructs a client against baseURL. A nil
// httpClient gets a client with a 30 second timeout that does not
// follow redirects, so the bearer token only goes to baseURL.
func NewFrostmolnClient(baseURL, tenantID, tokenFile string, httpClient *http.Client) *FrostmolnClient {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &FrostmolnClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		tenantID:   tenantID,
		tokenFile:  tokenFile,
		httpClient: httpClient,
	}
}

// NewFrostmolnClientFromEnv constructs a client from the operator-pod
// environment. The tenant is operator configuration only, never taken
// from a Database CR, so a CR cannot point the operator's identity at
// another tenant. The API URL and token file fall back to defaults.
func NewFrostmolnClientFromEnv() (*FrostmolnClient, error) {
	tenantID := os.Getenv(FrostmolnEnvTenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("frostmoln tenant ID not set: set the operator's %s environment variable", FrostmolnEnvTenantID)
	}
	return NewFrostmolnClient(
		envOrDefault(FrostmolnEnvAPIURL, FrostmolnDefaultAPIURL),
		tenantID,
		envOrDefault(FrostmolnEnvTokenFile, FrostmolnDefaultTokenFile),
		nil,
	), nil
}

func envOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// GetSecretValue returns the current value of the secret with the given
// name. A missing secret returns *SecretNotFoundError.
func (c *FrostmolnClient) GetSecretValue(ctx context.Context, name string) ([]byte, error) {
	token, err := c.readToken()
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/api/v1/tenants/%s/secrets/lookup?name=%s",
		c.baseURL, url.PathEscape(c.tenantID), url.QueryEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("frostmoln lookup request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("frostmoln lookup of secret %s failed: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Errors carry the status only, never a response body: they end up
	// in Database status and events, readable in the CR's namespace.
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, &SecretNotFoundError{SecretName: fmt.Sprintf("frostmoln://%s/%s", c.tenantID, name)}
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, fmt.Errorf("frostmoln lookup of secret %s rejected the workload identity token (HTTP 401): check the operator ServiceAccount annotation and that %s holds a current token", name, c.tokenFile)
	case resp.StatusCode == http.StatusForbidden:
		// The policy matches secrets by UUID, so a policy written with
		// the secret's name is the usual cause of a 403.
		return nil, fmt.Errorf("frostmoln lookup of secret %s refused (HTTP 403): check that the workload identity's IAM policy allows secrets:secrets:read on this secret's UUID", name)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return nil, fmt.Errorf("frostmoln lookup of secret %s failed (HTTP %d)", name, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, frostmolnMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("frostmoln lookup of secret %s: reading response: %w", name, err)
	}

	var parsed struct {
		SecretValue *string `json:"secretValue"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("frostmoln lookup of secret %s: parsing response: %w", name, err)
	}
	if parsed.SecretValue == nil {
		return nil, fmt.Errorf("frostmoln lookup of secret %s: response has no secretValue", name)
	}
	return []byte(*parsed.SecretValue), nil
}

func (c *FrostmolnClient) readToken() (string, error) {
	raw, err := os.ReadFile(c.tokenFile)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("frostmoln workload identity token %s not found: annotate the operator ServiceAccount with frostmoln.cloud/workload-identity-secret and mount that Secret's token key there (or set %s)", c.tokenFile, FrostmolnEnvTokenFile)
	}
	if err != nil {
		return "", fmt.Errorf("reading frostmoln workload identity token %s: %w", c.tokenFile, err)
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", fmt.Errorf("frostmoln workload identity token %s is empty", c.tokenFile)
	}
	return token, nil
}
