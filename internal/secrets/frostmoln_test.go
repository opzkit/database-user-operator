/*
Copyright 2026 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeToken(t *testing.T, path, token string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFrostmolnClient_GetSecretValue(t *testing.T) {
	var gotPath, gotQuery, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotAuth = r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"id":"0f85","name":"a b/c","secretValue":"postgres://admin@db/postgres"}`))
	}))
	defer srv.Close()

	tokenFile := filepath.Join(t.TempDir(), "token")
	writeToken(t, tokenFile, "tok-1\n")
	c := NewFrostmolnClient(srv.URL+"/", "tenant-1", tokenFile, nil)

	got, err := c.GetSecretValue(context.Background(), "a b/c")
	if err != nil {
		t.Fatalf("GetSecretValue: %v", err)
	}
	if string(got) != "postgres://admin@db/postgres" {
		t.Errorf("value = %q", got)
	}
	if gotPath != "/api/v1/tenants/tenant-1/secrets/lookup" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "name=a+b%2Fc" {
		t.Errorf("query = %q, want the name query-escaped", gotQuery)
	}
	if gotAuth != "Bearer tok-1" {
		t.Errorf("Authorization = %q, want the trimmed token", gotAuth)
	}

	// The add-on rotates the token; the next request must send the new one.
	writeToken(t, tokenFile, "tok-2")
	if _, err := c.GetSecretValue(context.Background(), "a b/c"); err != nil {
		t.Fatalf("GetSecretValue after rotation: %v", err)
	}
	if gotAuth != "Bearer tok-2" {
		t.Errorf("Authorization after rotation = %q, want Bearer tok-2", gotAuth)
	}
}

func TestFrostmolnClient_GetSecretValueErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
		notFind bool
	}{
		{name: "not found", status: http.StatusNotFound, body: `{"error":"not found"}`, notFind: true},
		{name: "forbidden", status: http.StatusForbidden, body: `{"error":"forbidden"}`, wantErr: "refused (HTTP 403)"},
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"error":"expired"}`, wantErr: "rejected the workload identity token (HTTP 401)"},
		{name: "server error", status: http.StatusBadGateway, body: "upstream down", wantErr: "failed (HTTP 502)"},
		{name: "redirect not followed", status: http.StatusFound, body: "", wantErr: "failed (HTTP 302)"},
		{name: "no secretValue", status: http.StatusOK, body: `{"id":"x"}`, wantErr: "no secretValue"},
		{name: "null secretValue", status: http.StatusOK, body: `{"secretValue":null}`, wantErr: "no secretValue"},
		{name: "not JSON", status: http.StatusOK, body: "<html>", wantErr: "parsing response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.status == http.StatusFound {
					w.Header().Set("Location", "/elsewhere")
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			tokenFile := filepath.Join(t.TempDir(), "token")
			writeToken(t, tokenFile, "tok")

			_, err := NewFrostmolnClient(srv.URL, "t", tokenFile, nil).GetSecretValue(context.Background(), "s")
			if tt.notFind {
				var nf *SecretNotFoundError
				if !errors.As(err, &nf) {
					t.Fatalf("error = %v (%T), want *SecretNotFoundError", err, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
			// Errors reach Database status and events: never echo a body.
			if tt.body != "" && strings.Contains(err.Error(), tt.body) {
				t.Errorf("error %q echoes the response body", err)
			}
		})
	}
}

func TestFrostmolnClient_Token(t *testing.T) {
	dir := t.TempDir()
	missing := NewFrostmolnClient("http://unused", "t", filepath.Join(dir, "absent"), nil)
	if _, err := missing.GetSecretValue(context.Background(), "s"); err == nil || !strings.Contains(err.Error(), "frostmoln.cloud/workload-identity-secret") {
		t.Errorf("missing token error = %v, want a hint about the ServiceAccount annotation", err)
	}

	empty := filepath.Join(dir, "empty")
	writeToken(t, empty, "  \n")
	if _, err := NewFrostmolnClient("http://unused", "t", empty, nil).GetSecretValue(context.Background(), "s"); err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Errorf("empty token error = %v", err)
	}
}

func TestNewFrostmolnClientFromEnv(t *testing.T) {
	t.Setenv(FrostmolnEnvTenantID, "")
	t.Setenv(FrostmolnEnvAPIURL, "")
	t.Setenv(FrostmolnEnvTokenFile, "")

	if _, err := NewFrostmolnClientFromEnv(); err == nil || !strings.Contains(err.Error(), FrostmolnEnvTenantID) {
		t.Errorf("no tenant error = %v", err)
	}

	t.Setenv(FrostmolnEnvTenantID, "from-env")
	c, err := NewFrostmolnClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.tenantID != "from-env" || c.baseURL != FrostmolnDefaultAPIURL || c.tokenFile != FrostmolnDefaultTokenFile {
		t.Errorf("defaults = %+v", c)
	}

	t.Setenv(FrostmolnEnvAPIURL, "https://api.example")
	t.Setenv(FrostmolnEnvTokenFile, "/tmp/tok")
	c, err = NewFrostmolnClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "https://api.example" || c.tokenFile != "/tmp/tok" {
		t.Errorf("env = %+v", c)
	}
}
