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
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newFakeClient(objs ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func sampleSecret() *DatabaseSecret {
	return &DatabaseSecret{
		DBHost:      "pg.example.com",
		DBPort:      5432,
		DBName:      "myapp_db",
		DBUsername:  "myapp_user",
		DBPassword:  "s3cret",
		DatabaseURL: "postgresql://myapp_user:s3cret@pg.example.com:5432/myapp_db",
		Engine:      "postgres",
	}
}

func TestKubernetesBackend_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient()
	b := NewKubernetesBackend(c, "test-ns")

	secret := sampleSecret()
	tags := map[string]string{"app": "myapp", "env": "test"}

	locator, version, err := b.Create(ctx, "myapp-creds", "myapp credentials", secret, tags, "")
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if locator != "test-ns/myapp-creds" {
		t.Errorf("locator = %q, want %q", locator, "test-ns/myapp-creds")
	}
	if version == "" {
		t.Error("expected non-empty resource version")
	}

	// Inspect the underlying Secret directly.
	var raw corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "myapp-creds"}, &raw); err != nil {
		t.Fatalf("failed to fetch underlying Secret: %v", err)
	}
	if raw.Type != corev1.SecretTypeOpaque {
		t.Errorf("Secret.Type = %q, want Opaque", raw.Type)
	}
	if raw.Annotations[DescriptionAnnotation] != "myapp credentials" {
		t.Errorf("description annotation missing or wrong: %q", raw.Annotations[DescriptionAnnotation])
	}
	if raw.Labels["app"] != "myapp" || raw.Labels["env"] != "test" {
		t.Errorf("labels = %v, want app=myapp env=test", raw.Labels)
	}

	// Round-trip the JSON payload.
	got, err := b.Get(ctx, "myapp-creds")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if got.DBPassword != "s3cret" || got.DBUsername != "myapp_user" {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func TestKubernetesBackend_CreateOverwritesExisting(t *testing.T) {
	ctx := context.Background()
	pre := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "myapp-creds", Namespace: "ns"},
		Data:       map[string][]byte{"stale": []byte("garbage")},
	}
	c := newFakeClient(pre)
	b := NewKubernetesBackend(c, "ns")

	if _, _, err := b.Create(ctx, "myapp-creds", "desc", sampleSecret(), nil, ""); err != nil {
		t.Fatalf("Create() over existing failed: %v", err)
	}

	var raw corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: "ns", Name: "myapp-creds"}, &raw); err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if _, ok := raw.Data[CredentialsKey]; !ok {
		t.Error("credentials key missing after upsert")
	}
}

func TestKubernetesBackend_ExistsAndUpdate(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient()
	b := NewKubernetesBackend(c, "ns")

	exists, err := b.Exists(ctx, "missing")
	if err != nil {
		t.Fatalf("Exists() failed on missing: %v", err)
	}
	if exists {
		t.Error("Exists() = true for missing secret")
	}

	if _, _, err := b.Create(ctx, "creds", "", sampleSecret(), nil, ""); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	exists, err = b.Exists(ctx, "creds")
	if err != nil {
		t.Fatalf("Exists() failed after create: %v", err)
	}
	if !exists {
		t.Error("Exists() = false after create")
	}

	updated := sampleSecret()
	updated.DBPassword = "new-password"
	if _, err := b.Update(ctx, "creds", updated, ""); err != nil {
		t.Fatalf("Update() failed: %v", err)
	}

	got, err := b.Get(ctx, "creds")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if got.DBPassword != "new-password" {
		t.Errorf("password not updated: got %q", got.DBPassword)
	}
}

func TestKubernetesBackend_UpdateMissingReturnsTypedError(t *testing.T) {
	ctx := context.Background()
	b := NewKubernetesBackend(newFakeClient(), "ns")

	_, err := b.Update(ctx, "no-such", sampleSecret(), "")
	var notFound *SecretNotFoundError
	if !errors.As(err, &notFound) {
		t.Errorf("expected *SecretNotFoundError, got %T: %v", err, err)
	}
}

func TestKubernetesBackend_DeleteIsIdempotent(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient()
	b := NewKubernetesBackend(c, "ns")

	if _, _, err := b.Create(ctx, "creds", "", sampleSecret(), nil, ""); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if err := b.Delete(ctx, "creds", false); err != nil {
		t.Fatalf("first Delete() failed: %v", err)
	}
	// Second delete on a missing secret should not error.
	if err := b.Delete(ctx, "creds", false); err != nil {
		t.Errorf("second Delete() returned error: %v", err)
	}
}

func TestKubernetesBackend_SyncTagsReplacesLabels(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient()
	b := NewKubernetesBackend(c, "ns")

	if _, _, err := b.Create(ctx, "creds", "", sampleSecret(), map[string]string{"old": "1", "keep": "yes"}, ""); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if err := b.SyncTags(ctx, "creds", map[string]string{"keep": "yes", "new": "2"}); err != nil {
		t.Fatalf("SyncTags() failed: %v", err)
	}

	var raw corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: "ns", Name: "creds"}, &raw); err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if _, stillThere := raw.Labels["old"]; stillThere {
		t.Error("removed label is still present")
	}
	if raw.Labels["new"] != "2" || raw.Labels["keep"] != "yes" {
		t.Errorf("labels = %v, want new=2 keep=yes", raw.Labels)
	}
}

func TestKubernetesBackend_GetMissingReturnsTypedError(t *testing.T) {
	ctx := context.Background()
	b := NewKubernetesBackend(newFakeClient(), "ns")

	_, err := b.Get(ctx, "no-such")
	var notFound *SecretNotFoundError
	if !errors.As(err, &notFound) {
		t.Errorf("expected *SecretNotFoundError, got %T: %v", err, err)
	}
}

func TestKubernetesBackend_PayloadRoundTrip(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient()
	b := NewKubernetesBackend(c, "ns")

	if _, _, err := b.Create(ctx, "creds", "", sampleSecret(), nil, ""); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	var raw corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: "ns", Name: "creds"}, &raw); err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	var blob map[string]any
	if err := json.Unmarshal(raw.Data[CredentialsKey], &blob); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	for _, key := range []string{"DB_HOST", "DB_PORT", "DB_NAME", "DB_USERNAME", "DB_PASSWORD", "POSTGRES_URL"} {
		if _, ok := blob[key]; !ok {
			t.Errorf("payload missing key %q (got %v)", key, blob)
		}
	}
}

// Sanity check: not-found errors from the fake client unwrap correctly.
func TestApierrorsSanity(t *testing.T) {
	err := apierrors.NewNotFound(corev1.Resource("secret"), "no-such")
	if !apierrors.IsNotFound(err) {
		t.Fatal("apierrors.NewNotFound did not produce IsNotFound err")
	}
}
