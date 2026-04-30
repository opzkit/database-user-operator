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

	infisical "github.com/infisical/go-sdk"
)

// fakeInfisicalClient is an in-memory implementation of the
// InfisicalSecretClient interface used for tests.
type fakeInfisicalClient struct {
	store map[string]string // key: <projectID>|<env>|<path>|<key>
}

func newFakeInfisicalClient() *fakeInfisicalClient {
	return &fakeInfisicalClient{store: map[string]string{}}
}

func (f *fakeInfisicalClient) k(projectID, env, path, key string) string {
	return projectID + "|" + env + "|" + path + "|" + key
}

func (f *fakeInfisicalClient) Create(opts infisical.CreateSecretOptions) (any, error) {
	k := f.k(opts.ProjectID, opts.Environment, opts.SecretPath, opts.SecretKey)
	if _, exists := f.store[k]; exists {
		return nil, errors.New("secret already exists")
	}
	f.store[k] = opts.SecretValue
	return nil, nil
}

func (f *fakeInfisicalClient) Retrieve(opts infisical.RetrieveSecretOptions) (string, error) {
	k := f.k(opts.ProjectID, opts.Environment, opts.SecretPath, opts.SecretKey)
	value, ok := f.store[k]
	if !ok {
		return "", errors.New("secret not found")
	}
	return value, nil
}

func (f *fakeInfisicalClient) Update(opts infisical.UpdateSecretOptions) (any, error) {
	k := f.k(opts.ProjectID, opts.Environment, opts.SecretPath, opts.SecretKey)
	if _, exists := f.store[k]; !exists {
		return nil, errors.New("secret not found")
	}
	f.store[k] = opts.NewSecretValue
	return nil, nil
}

func (f *fakeInfisicalClient) Delete(opts infisical.DeleteSecretOptions) (any, error) {
	k := f.k(opts.ProjectID, opts.Environment, opts.SecretPath, opts.SecretKey)
	if _, exists := f.store[k]; !exists {
		return nil, errors.New("secret not found")
	}
	delete(f.store, k)
	return nil, nil
}

func newTestBackend(client *fakeInfisicalClient) *InfisicalBackend {
	return newInfisicalBackendWithClient(
		client,
		"proj-uuid",
		"dev",
		"/",
		func(InfisicalAuth) error { return nil }, // login no-ops in tests
		InfisicalAuth{ClientID: "test-id", ClientSecret: "test-secret"},
	)
}

func TestInfisicalBackend_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	fc := newFakeInfisicalClient()
	b := newTestBackend(fc)

	loc, _, err := b.Create(ctx, "myapp_db", "creds", sampleSecret(), nil, "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if !strings.HasPrefix(loc, "infisical://proj-uuid/dev/") || !strings.HasSuffix(loc, "myapp_db") {
		t.Errorf("locator = %q", loc)
	}

	got, err := b.Get(ctx, "myapp_db")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.DBPassword != "s3cret" {
		t.Errorf("password mismatch: %+v", got)
	}
}

func TestInfisicalBackend_KeySanitisation(t *testing.T) {
	ctx := context.Background()
	fc := newFakeInfisicalClient()
	b := newTestBackend(fc)

	if _, _, err := b.Create(ctx, "rds/postgres/myapp_db", "", sampleSecret(), nil, ""); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Stored under sanitised key.
	if _, ok := fc.store["proj-uuid|dev|/|rds_postgres_myapp_db"]; !ok {
		t.Errorf("expected sanitised key in store, got %v", fc.store)
	}
}

func TestInfisicalBackend_CreateOverwritesExisting(t *testing.T) {
	ctx := context.Background()
	fc := newFakeInfisicalClient()
	b := newTestBackend(fc)

	if _, _, err := b.Create(ctx, "creds", "", sampleSecret(), nil, ""); err != nil {
		t.Fatalf("first Create failed: %v", err)
	}

	updated := sampleSecret()
	updated.DBPassword = "new-pwd"
	if _, _, err := b.Create(ctx, "creds", "", updated, nil, ""); err != nil {
		t.Fatalf("second Create (overwrite) failed: %v", err)
	}

	got, err := b.Get(ctx, "creds")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.DBPassword != "new-pwd" {
		t.Errorf("password not overwritten, got %q", got.DBPassword)
	}
}

func TestInfisicalBackend_UpdateMissingReturnsTypedError(t *testing.T) {
	ctx := context.Background()
	fc := newFakeInfisicalClient()
	b := newTestBackend(fc)

	_, err := b.Update(ctx, "no-such", sampleSecret(), "")
	var notFound *SecretNotFoundError
	if !errors.As(err, &notFound) {
		t.Errorf("expected *SecretNotFoundError, got %T: %v", err, err)
	}
}

func TestInfisicalBackend_GetMissingReturnsTypedError(t *testing.T) {
	ctx := context.Background()
	fc := newFakeInfisicalClient()
	b := newTestBackend(fc)

	_, err := b.Get(ctx, "no-such")
	var notFound *SecretNotFoundError
	if !errors.As(err, &notFound) {
		t.Errorf("expected *SecretNotFoundError, got %T: %v", err, err)
	}
}

func TestInfisicalBackend_DeleteIsIdempotent(t *testing.T) {
	ctx := context.Background()
	fc := newFakeInfisicalClient()
	b := newTestBackend(fc)

	if _, _, err := b.Create(ctx, "creds", "", sampleSecret(), nil, ""); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := b.Delete(ctx, "creds", false); err != nil {
		t.Fatalf("first Delete failed: %v", err)
	}
	if err := b.Delete(ctx, "creds", false); err != nil {
		t.Errorf("second Delete returned error: %v", err)
	}
}

func TestInfisicalBackend_ExistsTrueAfterCreate(t *testing.T) {
	ctx := context.Background()
	fc := newFakeInfisicalClient()
	b := newTestBackend(fc)

	exists, err := b.Exists(ctx, "creds")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("Exists = true for missing secret")
	}

	if _, _, err := b.Create(ctx, "creds", "", sampleSecret(), nil, ""); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	exists, err = b.Exists(ctx, "creds")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Exists = false after Create")
	}
}

func TestInfisicalBackend_PayloadIsJSON(t *testing.T) {
	ctx := context.Background()
	fc := newFakeInfisicalClient()
	b := newTestBackend(fc)

	if _, _, err := b.Create(ctx, "creds", "", sampleSecret(), nil, ""); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	raw, ok := fc.store["proj-uuid|dev|/|creds"]
	if !ok {
		t.Fatalf("expected entry, got %v", fc.store)
	}
	var blob map[string]any
	if err := json.Unmarshal([]byte(raw), &blob); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	for _, key := range []string{"DB_HOST", "DB_PORT", "DB_NAME", "DB_USERNAME", "DB_PASSWORD", "POSTGRES_URL"} {
		if _, ok := blob[key]; !ok {
			t.Errorf("payload missing %q", key)
		}
	}
}

func TestInfisicalBackend_LoginErrorPropagates(t *testing.T) {
	ctx := context.Background()
	fc := newFakeInfisicalClient()
	wantErr := errors.New("auth failed")
	b := newInfisicalBackendWithClient(
		fc, "p", "dev", "/",
		func(InfisicalAuth) error { return wantErr },
		InfisicalAuth{ClientID: "x", ClientSecret: "y"},
	)

	_, err := b.Exists(ctx, "anything")
	if err == nil || !strings.Contains(err.Error(), "auth failed") {
		t.Errorf("expected wrapped login error, got %v", err)
	}
}

func TestInfisicalBackend_SyncTagsIsNoOp(t *testing.T) {
	ctx := context.Background()
	b := newTestBackend(newFakeInfisicalClient())
	if err := b.SyncTags(ctx, "any", map[string]string{"a": "1"}); err != nil {
		t.Errorf("SyncTags returned error: %v", err)
	}
}
