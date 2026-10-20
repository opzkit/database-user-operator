/*
Copyright 2026 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	databasev1alpha1 "opzkit/database-user-operator/api/v1alpha1"
	"opzkit/database-user-operator/internal/secrets"
)

// frostmolnDB returns a Database whose admin DSN comes from the Frostmoln
// secret name, with the operator env pointed at a fake lookup API that
// serves value for that name.
func frostmolnDB(t *testing.T, name, value string) *databasev1alpha1.Database {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != name {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"secretValue": value})
	}))
	t.Cleanup(srv.Close)

	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("token"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(secrets.FrostmolnEnvAPIURL, srv.URL)
	t.Setenv(secrets.FrostmolnEnvTokenFile, tokenFile)
	t.Setenv(secrets.FrostmolnEnvTenantID, "tenant-1")

	return &databasev1alpha1.Database{
		Spec: databasev1alpha1.DatabaseSpec{
			DatabaseName: "app",
			ConnectionString: databasev1alpha1.ConnectionStringSource{
				Frostmoln: &databasev1alpha1.FrostmolnConnectionStringRef{Name: name},
			},
		},
	}
}

func TestGetConnectionString_Frostmoln(t *testing.T) {
	const dsn = "postgres://admin:p%40ss@172.24.16.10:5432/postgres?sslmode=require"

	tests := []struct {
		name    string
		value   string
		key     string
		want    string
		wantErr string
	}{
		{name: "raw DSN", value: dsn, want: dsn},
		{name: "JSON default key", value: `{"connectionString":"` + dsn + `"}`, want: dsn},
		{name: "JSON custom key", value: `{"url":"` + dsn + `"}`, key: "url", want: dsn},
		{name: "JSON missing key", value: `{"url":"` + dsn + `"}`, wantErr: "key connectionString not found in Frostmoln secret admin"},
		{name: "empty value", value: "", wantErr: "connection string is empty in Frostmoln secret admin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := frostmolnDB(t, "admin", tt.value)
			db.Spec.ConnectionString.Frostmoln.Key = tt.key

			got, err := (&DatabaseReconciler{}).getConnectionString(context.Background(), db)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("getConnectionString: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetConnectionString_FrostmolnMissingSecret(t *testing.T) {
	db := frostmolnDB(t, "admin", "x")
	db.Spec.ConnectionString.Frostmoln.Name = "other"

	_, err := (&DatabaseReconciler{}).getConnectionString(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %v, want a not-found error", err)
	}
}

func TestConnectionStringFromPayload_RawWithBrace(t *testing.T) {
	// A raw DSN whose password contains { must not be parsed as JSON.
	const dsn = "postgres://admin:a{b@host:5432/postgres"
	got, err := connectionStringFromPayload([]byte(dsn), "connectionString", "test secret")
	if err != nil {
		t.Fatalf("connectionStringFromPayload: %v", err)
	}
	if got != dsn {
		t.Errorf("got %q, want %q", got, dsn)
	}
}
