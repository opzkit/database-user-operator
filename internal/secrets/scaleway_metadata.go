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
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

// Scaleway exposes per-instance metadata over a link-local HTTP endpoint
// reachable from the node (and from pods sharing the node network). We
// query it to auto-derive the operator's home Project and region so a
// Scaleway-hosted operator needs no explicit SCW_DEFAULT_* configuration.
const (
	scalewayMetadataURL     = "http://169.254.42.42"
	scalewayMetadataTimeout = 2 * time.Second
	scalewayMetadataMaxBody = 1 << 20 // 1 MiB cap on the metadata response
)

// scalewayInstanceMetadata is the subset of the /conf payload we read.
// Scaleway returns far more; json.Unmarshal ignores the rest.
type scalewayInstanceMetadata struct {
	Project  string `json:"project"`
	Location struct {
		ZoneID string `json:"zone_id"`
	} `json:"location"`
}

// fetchScalewayInstanceMetadata GETs /conf?format=json from baseURL and
// decodes the Project + zone. It uses the supplied client (which must
// carry its own timeout) and a context deadline — it never mutates
// http.DefaultClient, unlike the Scaleway SDK's metadata helper.
func fetchScalewayInstanceMetadata(ctx context.Context, httpClient *http.Client, baseURL string) (*scalewayInstanceMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/conf?format=json", nil)
	if err != nil {
		return nil, fmt.Errorf("scaleway metadata: build request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scaleway metadata: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scaleway metadata: unexpected status %d", resp.StatusCode)
	}

	var meta scalewayInstanceMetadata
	if err := json.NewDecoder(io.LimitReader(resp.Body, scalewayMetadataMaxBody)).Decode(&meta); err != nil {
		return nil, fmt.Errorf("scaleway metadata: decode response: %w", err)
	}
	return &meta, nil
}

// scalewaySecretManagerRegions is the set of regions Scaleway Secret
// Manager supports — mirrors the CRD region enum. scw.ParseRegion only
// validates the format (it accepts any well-formed "xx-yyy"), so we
// gate metadata-derived regions on membership here.
var scalewaySecretManagerRegions = map[string]struct{}{
	"fr-par": {},
	"nl-ams": {},
	"pl-waw": {},
}

// zoneToRegion derives a region from a Scaleway zone id (e.g. fr-par-1 ->
// fr-par) and validates it against the supported region set.
func zoneToRegion(zoneID string) (string, error) {
	parts := strings.Split(zoneID, "-")
	if len(parts) < 2 {
		return "", fmt.Errorf("scaleway metadata: cannot derive region from zone %q", zoneID)
	}
	region := parts[0] + "-" + parts[1]
	if _, ok := scalewaySecretManagerRegions[region]; !ok {
		return "", fmt.Errorf("scaleway metadata: region %q derived from zone %q is not a supported Secret Manager region", region, zoneID)
	}
	return region, nil
}

// ApplyScalewayMetadataDefaults best-effort populates the
// SCW_DEFAULT_PROJECT_ID / SCW_DEFAULT_REGION env vars from the node's
// Scaleway instance metadata, but only for vars that aren't already set.
//
// Precedence is preserved: an explicitly configured operator env var
// always wins over metadata. Combined with per-CR spec fields the full
// order is CR field -> operator env -> metadata-derived env -> error.
//
// It is intentionally non-fatal: when metadata is unreachable (the
// operator isn't on Scaleway, or a CNI blocks the link-local address)
// it logs at V(1) and returns, leaving resolution to fall through to the
// existing error path.
func ApplyScalewayMetadataDefaults(ctx context.Context, logger logr.Logger) {
	needProject := os.Getenv(ScalewayEnvDefaultProjectID) == ""
	needRegion := os.Getenv(ScalewayEnvDefaultRegion) == ""
	if !needProject && !needRegion {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, scalewayMetadataTimeout)
	defer cancel()

	meta, err := fetchScalewayInstanceMetadata(ctx, &http.Client{Timeout: scalewayMetadataTimeout}, scalewayMetadataURL)
	if err != nil {
		logger.V(1).Info("Scaleway instance metadata unavailable; skipping operator-default auto-detection", "error", err.Error())
		return
	}

	if needProject {
		if meta.Project == "" {
			logger.V(1).Info("Scaleway instance metadata returned no project; leaving SCW_DEFAULT_PROJECT_ID unset")
		} else {
			_ = os.Setenv(ScalewayEnvDefaultProjectID, meta.Project)
			logger.Info("Derived Scaleway default projectID from instance metadata", "projectID", meta.Project)
		}
	}

	if needRegion {
		region, err := zoneToRegion(meta.Location.ZoneID)
		if err != nil {
			logger.V(1).Info("Could not derive Scaleway default region from instance metadata", "zoneID", meta.Location.ZoneID, "error", err.Error())
		} else {
			_ = os.Setenv(ScalewayEnvDefaultRegion, region)
			logger.Info("Derived Scaleway default region from instance metadata", "region", region, "zoneID", meta.Location.ZoneID)
		}
	}
}
