/*
Copyright 2026 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import (
	"context"
	"sort"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getSecretData returns the data of Secret ns/app-db, which every test here writes.
func getSecretData(t *testing.T, c client.Client) map[string][]byte {
	t.Helper()
	var s corev1.Secret
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "app-db"}, &s); err != nil {
		t.Fatalf("Get app-db: %v", err)
	}
	return s.Data
}

func dataKeys(data map[string][]byte) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

func TestKubernetesBackend_FieldKeysDefaultFormat(t *testing.T) {
	c := newFakeClient()
	b := NewKubernetesBackend(c, "ns", WithFieldKeys())

	if _, _, err := b.Create(context.Background(), "app-db", "desc", sampleSecret(), nil, ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	data := getSecretData(t, c)

	if got, want := dataKeys(data), "DB_HOST,DB_NAME,DB_PASSWORD,DB_PORT,DB_USERNAME,POSTGRES_URL,credentials"; got != want {
		t.Fatalf("keys = %s, want %s", got, want)
	}
	for key, want := range map[string]string{
		"DB_HOST":      "pg.example.com",
		"DB_PORT":      "5432",
		"DB_PASSWORD":  "s3cret",
		"POSTGRES_URL": "postgresql://myapp_user:s3cret@pg.example.com:5432/myapp_db",
	} {
		if string(data[key]) != want {
			t.Errorf("%s = %q, want %q", key, data[key], want)
		}
	}

	// Get, and with it the password reuse in format migrations, still
	// reads the JSON under "credentials".
	got, err := b.Get(context.Background(), "app-db")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DBPassword != "s3cret" {
		t.Errorf("Get password = %q", got.DBPassword)
	}
}

func TestKubernetesBackend_FieldKeysTemplate(t *testing.T) {
	c := newFakeClient()
	b := NewKubernetesBackend(c, "ns", WithFieldKeys())
	tmpl := `{"spring.datasource.url": "jdbc:postgresql://{{.DBHost}}:{{.DBPort}}/{{.DBName}}", "spring.datasource.password": "{{.DBPassword}}"}`

	if _, _, err := b.Create(context.Background(), "app-db", "desc", sampleSecret(), nil, tmpl); err != nil {
		t.Fatalf("Create: %v", err)
	}
	data := getSecretData(t, c)
	if got, want := dataKeys(data), "credentials,spring.datasource.password,spring.datasource.url"; got != want {
		t.Fatalf("keys = %s, want %s", got, want)
	}
	if got := string(data["spring.datasource.url"]); got != "jdbc:postgresql://pg.example.com:5432/myapp_db" {
		t.Errorf("spring.datasource.url = %q", got)
	}
}

func TestKubernetesBackend_FieldKeysValues(t *testing.T) {
	c := newFakeClient()
	b := NewKubernetesBackend(c, "ns", WithFieldKeys())
	tmpl := `{"PORT": {{.DBPort}}, "RATIO": 5.432e3, "TLS": true, "EMPTY": "", "NOTE": "café \"q\" \\ /"}`

	if _, _, err := b.Create(context.Background(), "app-db", "desc", sampleSecret(), nil, tmpl); err != nil {
		t.Fatalf("Create: %v", err)
	}
	data := getSecretData(t, c)
	for key, want := range map[string]string{
		"PORT":  "5432",
		"RATIO": "5.432e3", // numbers are kept as written, not reformatted
		"TLS":   "true",
		"EMPTY": "",
		"NOTE":  `café "q" \ /`,
	} {
		got, ok := data[key]
		if !ok {
			t.Errorf("%s missing", key)
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// With fieldKeys the operator owns the data: a key it no longer writes
// (here one left over from an older template) must not keep an old value.
func TestKubernetesBackend_FieldKeysReplaceStaleKeys(t *testing.T) {
	for _, op := range []string{"create over existing", "update"} {
		t.Run(op, func(t *testing.T) {
			pre := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "app-db", Namespace: "ns"},
				Data: map[string][]byte{
					CredentialsKey: []byte(`{"OLD_PASSWORD":"old"}`),
					"OLD_PASSWORD": []byte("old"),
				},
			}
			c := newFakeClient(pre)
			b := NewKubernetesBackend(c, "ns", WithFieldKeys())

			var err error
			if op == "update" {
				_, err = b.Update(context.Background(), "app-db", sampleSecret(), "")
			} else {
				_, _, err = b.Create(context.Background(), "app-db", "desc", sampleSecret(), nil, "")
			}
			if err != nil {
				t.Fatalf("%s: %v", op, err)
			}
			if _, ok := getSecretData(t, c)["OLD_PASSWORD"]; ok {
				t.Error("stale key OLD_PASSWORD kept")
			}
		})
	}
}

// Without fieldKeys the Secret keeps its single-key shape and keys the
// operator does not own are left alone, as before.
func TestKubernetesBackend_NoFieldKeysUnchanged(t *testing.T) {
	pre := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "app-db", Namespace: "ns"},
		Data:       map[string][]byte{"extra": []byte("kept")},
	}
	c := newFakeClient(pre)
	b := NewKubernetesBackend(c, "ns")

	if _, _, err := b.Create(context.Background(), "app-db", "desc", sampleSecret(), nil, ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got, want := dataKeys(getSecretData(t, c)), "credentials,extra"; got != want {
		t.Errorf("keys = %s, want %s", got, want)
	}
}

func TestKubernetesBackend_FieldKeysRejects(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		wantErr string
	}{
		{"null", `{"TOKEN": null}`, `field "TOKEN"`},
		// Several bad fields: always the first by name, so the status is stable
		{"several bad fields", `{"zeta": null, "alpha": ["x"], "mid": {"a": 1}}`, `field "alpha"`},
		{"nested object", `{"db": {"password": "{{.DBPassword}}"}}`, `field "db"`},
		{"array", `{"hosts": ["{{.DBHost}}"]}`, `field "hosts"`},
		{"invalid key", `{"bad key": "{{.DBPassword}}"}`, `field "bad key" is not a valid Secret key`},
		{"credentials collision", `{"credentials": "{{.DBPassword}}"}`, `collides`},
		{"not an object", `"{{.DBPassword}}"`, `JSON object`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewKubernetesBackend(newFakeClient(), "ns", WithFieldKeys())
			_, _, err := b.Create(context.Background(), "app-db", "desc", sampleSecret(), nil, tt.tmpl)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
			// Errors reach Database status and events: never the value.
			if strings.Contains(err.Error(), "s3cret") {
				t.Errorf("error %q contains the password", err)
			}
		})
	}
}
