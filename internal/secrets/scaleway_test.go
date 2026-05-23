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
	"strings"
	"testing"

	smapi "github.com/scaleway/scaleway-sdk-go/api/secret/v1beta1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// fakeScalewayClient is an in-memory implementation of ScalewaySMClient
// used in tests. It models the subset of API behaviour the backend
// relies on: name+project lookup, version creation, latest_enabled
// access, tag/description update, and delete.
type fakeScalewayClient struct {
	secrets      map[string]*smapi.Secret // keyed by secret ID
	versions     map[string][][]byte      // keyed by secret ID; index = revision-1
	createErr    error
	listErr      error
	updateErr    error
	deleteErr    error
	createVerErr error
	accessVerErr error
	idCounter    int
	createdCalls int
	versionCalls int
	updatedCalls int
	deletedCalls int
	listCalls    int
}

func newFakeScalewayClient() *fakeScalewayClient {
	return &fakeScalewayClient{
		secrets:  map[string]*smapi.Secret{},
		versions: map[string][][]byte{},
	}
}

func (f *fakeScalewayClient) ListSecrets(req *smapi.ListSecretsRequest, _ ...scw.RequestOption) (*smapi.ListSecretsResponse, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := &smapi.ListSecretsResponse{}
	for _, s := range f.secrets {
		if req.Name != nil && s.Name != *req.Name {
			continue
		}
		if req.ProjectID != nil && s.ProjectID != *req.ProjectID {
			continue
		}
		if req.Path != nil && s.Path != *req.Path {
			continue
		}
		out.Secrets = append(out.Secrets, s)
	}
	out.TotalCount = uint64(len(out.Secrets))
	return out, nil
}

func (f *fakeScalewayClient) CreateSecret(req *smapi.CreateSecretRequest, _ ...scw.RequestOption) (*smapi.Secret, error) {
	f.createdCalls++
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.idCounter++
	id := "secret-id-" + itoa(f.idCounter)
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}
	// Mirror Scaleway Secret Manager's server-side normalisation:
	// trailing `/` is stripped from non-root paths. Tests rely on this
	// to catch the round-trip bug where Create wrote `/rds/postgres/`
	// but ListSecrets later returned `/rds/postgres`.
	path := "/"
	if req.Path != nil {
		path = strings.TrimRight(*req.Path, "/")
		if path == "" {
			path = "/"
		}
	}
	s := &smapi.Secret{
		ID:          id,
		ProjectID:   req.ProjectID,
		Name:        req.Name,
		Path:        path,
		Tags:        append([]string(nil), req.Tags...),
		Description: &desc,
		Region:      req.Region,
	}
	f.secrets[id] = s
	return s, nil
}

func (f *fakeScalewayClient) UpdateSecret(req *smapi.UpdateSecretRequest, _ ...scw.RequestOption) (*smapi.Secret, error) {
	f.updatedCalls++
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	s, ok := f.secrets[req.SecretID]
	if !ok {
		return nil, errors.New("not found")
	}
	if req.Tags != nil {
		s.Tags = append([]string(nil), (*req.Tags)...)
	}
	if req.Description != nil {
		d := *req.Description
		s.Description = &d
	}
	return s, nil
}

func (f *fakeScalewayClient) DeleteSecret(req *smapi.DeleteSecretRequest, _ ...scw.RequestOption) error {
	f.deletedCalls++
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.secrets, req.SecretID)
	delete(f.versions, req.SecretID)
	return nil
}

func (f *fakeScalewayClient) CreateSecretVersion(req *smapi.CreateSecretVersionRequest, _ ...scw.RequestOption) (*smapi.SecretVersion, error) {
	f.versionCalls++
	if f.createVerErr != nil {
		return nil, f.createVerErr
	}
	if _, ok := f.secrets[req.SecretID]; !ok {
		return nil, errors.New("secret not found")
	}
	f.versions[req.SecretID] = append(f.versions[req.SecretID], append([]byte(nil), req.Data...))
	rev := uint32(len(f.versions[req.SecretID]))
	return &smapi.SecretVersion{Revision: rev, SecretID: req.SecretID}, nil
}

func (f *fakeScalewayClient) AccessSecretVersion(req *smapi.AccessSecretVersionRequest, _ ...scw.RequestOption) (*smapi.AccessSecretVersionResponse, error) {
	if f.accessVerErr != nil {
		return nil, f.accessVerErr
	}
	versions, ok := f.versions[req.SecretID]
	if !ok || len(versions) == 0 {
		return nil, errors.New("no versions")
	}
	// "latest" / "latest_enabled" / numeric — we treat them all as latest in the fake.
	return &smapi.AccessSecretVersionResponse{
		SecretID: req.SecretID,
		Data:     append([]byte(nil), versions[len(versions)-1]...),
	}, nil
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

func newScalewayTestBackend(client ScalewaySMClient) *ScalewayBackend {
	return newScalewayBackendWithClient(client, scw.RegionFrPar, "proj-uuid")
}

func sampleDBSecret() *DatabaseSecret {
	return &DatabaseSecret{
		DBHost:      "pg.example.com",
		DBPort:      5432,
		DBName:      "appdb",
		DBUsername:  "appuser",
		DBPassword:  "s3cret",
		DatabaseURL: "postgresql://appuser:s3cret@pg.example.com:5432/appdb",
		Engine:      "postgres",
	}
}

func TestScalewayBackend_Create_NewSecret(t *testing.T) {
	fake := newFakeScalewayClient()
	b := newScalewayTestBackend(fake)
	tags := map[string]string{"env": "test", "owner": "dbuo"}

	loc, ver, err := b.Create(context.Background(), "rds/postgres/foo", "test", sampleDBSecret(), tags, "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if loc != "scaleway://fr-par/proj-uuid/rds/postgres/foo" {
		t.Errorf("locator = %q", loc)
	}
	for _, s := range fake.secrets {
		if s.Name != "foo" || s.Path != "/rds/postgres" {
			t.Errorf("(name, path) = (%q, %q), want (\"foo\", \"/rds/postgres\")", s.Name, s.Path)
		}
	}
	if ver != "1" {
		t.Errorf("version = %q, want 1", ver)
	}
	if fake.createdCalls != 1 || fake.versionCalls != 1 {
		t.Errorf("expected 1 create + 1 version, got %d/%d", fake.createdCalls, fake.versionCalls)
	}
	// Tags serialised as key=value, sorted.
	for _, s := range fake.secrets {
		if got, want := strings.Join(s.Tags, ","), "env=test,owner=dbuo"; got != want {
			t.Errorf("tags = %q, want %q", got, want)
		}
	}
}

func TestScalewayBackend_Create_RestoreOnExisting(t *testing.T) {
	fake := newFakeScalewayClient()
	b := newScalewayTestBackend(fake)

	// Pre-seed a Secret to simulate an already-existing entry.
	if _, _, err := b.Create(context.Background(), "rds/postgres/foo", "first", sampleDBSecret(), nil, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Second Create on the same name must NOT create a second Secret;
	// it should add a new version + push tag/description updates.
	updatedTags := map[string]string{"env": "prod"}
	_, ver, err := b.Create(context.Background(), "rds/postgres/foo", "second", sampleDBSecret(), updatedTags, "")
	if err != nil {
		t.Fatalf("Create-on-existing: %v", err)
	}
	if ver != "2" {
		t.Errorf("version = %q, want 2", ver)
	}
	if fake.createdCalls != 1 {
		t.Errorf("CreateSecret called %d times, want 1 (second call should reuse existing)", fake.createdCalls)
	}
	if fake.updatedCalls != 1 {
		t.Errorf("UpdateSecret called %d times, want 1 (description+tags refresh)", fake.updatedCalls)
	}
	if fake.versionCalls != 2 {
		t.Errorf("CreateSecretVersion called %d times, want 2", fake.versionCalls)
	}
}

func TestScalewayBackend_Get_RoundTrip(t *testing.T) {
	fake := newFakeScalewayClient()
	b := newScalewayTestBackend(fake)

	want := sampleDBSecret()
	if _, _, err := b.Create(context.Background(), "myapp", "", want, nil, ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := b.Get(context.Background(), "myapp")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// DatabaseURL/Engine aren't round-tripped (they're derived); spot-check the persisted fields.
	if got.DBHost != want.DBHost || got.DBPassword != want.DBPassword || got.DBPort != want.DBPort {
		t.Errorf("round-trip mismatch: %+v vs %+v", got, want)
	}
}

func TestScalewayBackend_Get_NotFound(t *testing.T) {
	b := newScalewayTestBackend(newFakeScalewayClient())
	_, err := b.Get(context.Background(), "missing")
	var nf *SecretNotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected SecretNotFoundError, got %v", err)
	}
}

func TestScalewayBackend_Update_NewVersion(t *testing.T) {
	fake := newFakeScalewayClient()
	b := newScalewayTestBackend(fake)
	if _, _, err := b.Create(context.Background(), "x", "", sampleDBSecret(), nil, ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	rotated := sampleDBSecret()
	rotated.DBPassword = "rotated"
	ver, err := b.Update(context.Background(), "x", rotated, "")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if ver != "2" {
		t.Errorf("version = %q, want 2", ver)
	}
	got, err := b.Get(context.Background(), "x")
	if err != nil {
		t.Fatalf("Get post-update: %v", err)
	}
	if got.DBPassword != "rotated" {
		t.Errorf("password = %q, want rotated", got.DBPassword)
	}
}

func TestScalewayBackend_Update_NotFound(t *testing.T) {
	b := newScalewayTestBackend(newFakeScalewayClient())
	_, err := b.Update(context.Background(), "missing", sampleDBSecret(), "")
	var nf *SecretNotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected SecretNotFoundError, got %v", err)
	}
}

func TestScalewayBackend_Delete(t *testing.T) {
	fake := newFakeScalewayClient()
	b := newScalewayTestBackend(fake)
	if _, _, err := b.Create(context.Background(), "x", "", sampleDBSecret(), nil, ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := b.Delete(context.Background(), "x", false); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if fake.deletedCalls != 1 {
		t.Errorf("DeleteSecret called %d times, want 1", fake.deletedCalls)
	}
	if ok, _ := b.Exists(context.Background(), "x"); ok {
		t.Errorf("secret still exists post-delete")
	}
	// Delete on non-existent → no error.
	if err := b.Delete(context.Background(), "missing", false); err != nil {
		t.Errorf("Delete on missing: %v", err)
	}
}

func TestScalewayBackend_SyncTags(t *testing.T) {
	fake := newFakeScalewayClient()
	b := newScalewayTestBackend(fake)
	initial := map[string]string{"a": "1"}
	if _, _, err := b.Create(context.Background(), "x", "", sampleDBSecret(), initial, ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	desired := map[string]string{"b": "2", "c": "3"}
	if err := b.SyncTags(context.Background(), "x", desired); err != nil {
		t.Fatalf("SyncTags: %v", err)
	}
	for _, s := range fake.secrets {
		got := strings.Join(s.Tags, ",")
		want := "b=2,c=3"
		if got != want {
			t.Errorf("tags after SyncTags = %q, want %q", got, want)
		}
	}
}

func TestScalewayBackend_SyncTags_NotFound(t *testing.T) {
	b := newScalewayTestBackend(newFakeScalewayClient())
	err := b.SyncTags(context.Background(), "missing", map[string]string{"a": "1"})
	var nf *SecretNotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected SecretNotFoundError, got %v", err)
	}
}

func TestScalewayBackend_PathAndNameSplit(t *testing.T) {
	cases := []struct {
		in       string
		wantPath string
		wantName string
	}{
		{"rds/postgres/foo", "/rds/postgres", "foo"},
		{"/rds/postgres/foo", "/rds/postgres", "foo"},
		{"/rds/postgres/foo/", "/rds/postgres", "foo"},
		{"foo", "/", "foo"},
		{"", "/", ""},
		{"/", "/", ""},
		{"a/b", "/a", "b"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			gotPath, gotName := scalewayPathAndName(tc.in)
			if gotPath != tc.wantPath || gotName != tc.wantName {
				t.Errorf("scalewayPathAndName(%q) = (%q, %q), want (%q, %q)",
					tc.in, gotPath, gotName, tc.wantPath, tc.wantName)
			}
		})
	}
}

func TestScalewayBackend_Create_WritesAtPath(t *testing.T) {
	fake := newFakeScalewayClient()
	b := newScalewayTestBackend(fake)
	if _, _, err := b.Create(context.Background(), "/rds/postgres/myapp/", "", sampleDBSecret(), nil, ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(fake.secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(fake.secrets))
	}
	for _, s := range fake.secrets {
		if s.Name != "myapp" {
			t.Errorf("name = %q, want myapp", s.Name)
		}
		if s.Path != "/rds/postgres" {
			t.Errorf("path = %q, want /rds/postgres", s.Path)
		}
	}
}

func TestScalewayBackend_GetRawAt(t *testing.T) {
	fake := newFakeScalewayClient()
	b := newScalewayTestBackend(fake)

	// Seed a Scaleway Secret at /rdb/postgres, name intersolia-staging-pg
	// holding a JSON payload that mirrors what terraform/scaleway would
	// produce for a Managed Postgres admin secret.
	payload := []byte(`{"DB_HOST":"pg.example.com","DB_PORT":"5432","DB_NAME":"rdb","DB_USERNAME":"intersolia_admin","DB_PASSWORD":"s3cret"}`)
	fake.idCounter++
	id := "secret-id-admin"
	fake.secrets[id] = &smapi.Secret{
		ID:        id,
		ProjectID: "proj-uuid",
		Name:      "intersolia-staging-pg",
		Path:      "/rdb/postgres",
		Region:    scw.RegionFrPar,
	}
	fake.versions[id] = [][]byte{payload}

	got, err := b.GetRawAt(context.Background(), "/rdb/postgres", "intersolia-staging-pg")
	if err != nil {
		t.Fatalf("GetRawAt: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("GetRawAt payload mismatch:\ngot:  %s\nwant: %s", got, payload)
	}

	// Miss path: nonexistent (path, name) must return SecretNotFoundError.
	_, err = b.GetRawAt(context.Background(), "/rdb/postgres", "does-not-exist")
	var nf *SecretNotFoundError
	if !errors.As(err, &nf) {
		t.Errorf("GetRawAt on missing secret: want *SecretNotFoundError, got %T (%v)", err, err)
	}

	// Trailing-slash tolerance: a caller-supplied "/rdb/postgres/"
	// must canonicalise to "/rdb/postgres" before the ListSecrets
	// filter, otherwise the round-trip bug class fixed in #172
	// resurfaces on the read side.
	got, err = b.GetRawAt(context.Background(), "/rdb/postgres/", "intersolia-staging-pg")
	if err != nil {
		t.Fatalf("GetRawAt with trailing-slash path: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("GetRawAt trailing-slash payload mismatch:\ngot:  %s\nwant: %s", got, payload)
	}
}

// Regression: on a second reconcile the existence check has to find
// the Secret the operator wrote on the first reconcile, otherwise the
// operator loops forever attempting CreateSecret and Scaleway returns
// "name is wrongly formatted, cannot have same secret name in same
// path". Pre-fix the path-and-name split produced "/rds/postgres/"
// (trailing slash) for both the Create request and the subsequent
// find filter; Scaleway normalised the stored Path to "/rds/postgres"
// and the in-code `s.Path == path` comparison rejected the hit.
// Originally added in #172; kept here so the round-trip stays
// guarded independently of GetRawAt coverage.
func TestScalewayBackend_FindAfterCreate(t *testing.T) {
	fake := newFakeScalewayClient()
	b := newScalewayTestBackend(fake)

	if _, _, err := b.Create(context.Background(), "rds/postgres/chemcat", "", sampleDBSecret(), nil, ""); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ok, err := b.Exists(context.Background(), "rds/postgres/chemcat")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !ok {
		t.Fatalf("Exists after Create returned false — operator would loop on CreateSecret")
	}

	got, err := b.Get(context.Background(), "rds/postgres/chemcat")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DBHost != "pg.example.com" {
		t.Errorf("Get returned wrong payload: DBHost = %q", got.DBHost)
	}
}

func TestScalewayBackend_LegacyReadFallback(t *testing.T) {
	fake := newFakeScalewayClient()
	b := newScalewayTestBackend(fake)

	// Seed the fake with a legacy secret (root path, flattened name)
	// as a pre-path operator release would have written it.
	legacyName := "rds_postgres_legacy"
	fake.idCounter++
	id := "secret-id-legacy"
	fake.secrets[id] = &smapi.Secret{
		ID:        id,
		ProjectID: "proj-uuid",
		Name:      legacyName,
		Path:      "/",
		Region:    scw.RegionFrPar,
	}
	fake.versions[id] = [][]byte{mustJSON(t, sampleDBSecret())}

	// Lookup using the path-style logical name should hit the legacy
	// secret via the read-side fallback.
	got, err := b.Get(context.Background(), "rds/postgres/legacy")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DBHost != "pg.example.com" {
		t.Errorf("DBHost = %q, want pg.example.com", got.DBHost)
	}
}

func mustJSON(t *testing.T, s *DatabaseSecret) []byte {
	t.Helper()
	b, err := s.ToJSONWithTemplate("")
	if err != nil {
		t.Fatalf("ToJSONWithTemplate: %v", err)
	}
	return b
}

func TestScalewayBackend_NewBackend_Validation(t *testing.T) {
	cases := []struct {
		name   string
		region string
		proj   string
		auth   ScalewayAuth
		errSub string
	}{
		{"empty region", "", "p", ScalewayAuth{AccessKey: "a", SecretKey: "s"}, "region is required"},
		{"invalid region", "mars-1", "p", ScalewayAuth{AccessKey: "a", SecretKey: "s"}, "invalid scaleway region"},
		{"empty project", "fr-par", "", ScalewayAuth{AccessKey: "a", SecretKey: "s"}, "projectID is required"},
		{"empty auth", "fr-par", "p", ScalewayAuth{}, "AccessKey and auth.SecretKey"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewScalewayBackend(tc.region, tc.proj, tc.auth)
			if err == nil || !strings.Contains(err.Error(), tc.errSub) {
				t.Errorf("err = %v, want substring %q", err, tc.errSub)
			}
		})
	}
}

// Sanity-check the JSON shape produced by Create matches what
// downstream consumers (e.g. ExternalSecrets templates) would expect:
// the same DatabaseSecret JSON the AWS / K8s / Infisical backends
// store.
func TestScalewayBackend_StoredPayloadIsDatabaseSecretJSON(t *testing.T) {
	fake := newFakeScalewayClient()
	b := newScalewayTestBackend(fake)
	if _, _, err := b.Create(context.Background(), "x", "", sampleDBSecret(), nil, ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for id := range fake.versions {
		raw := fake.versions[id][0]
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("stored payload is not valid JSON: %v", err)
		}
		for _, k := range []string{"DB_HOST", "DB_PORT", "DB_NAME", "DB_USERNAME", "DB_PASSWORD", "POSTGRES_URL"} {
			if _, ok := m[k]; !ok {
				t.Errorf("stored payload missing key %q: %v", k, m)
			}
		}
	}
}
