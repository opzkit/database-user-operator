/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchScalewayInstanceMetadata(t *testing.T) {
	const body = `{
		"id": "11111111-1111-1111-1111-111111111111",
		"project": "5a339eb9-920f-4256-bbf2-d9ae6e0fb676",
		"location": {"zone_id": "fr-par-1", "node_id": "x"},
		"tags": ["ignored"]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/conf" || r.URL.Query().Get("format") != "json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	meta, err := fetchScalewayInstanceMetadata(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("fetchScalewayInstanceMetadata: %v", err)
	}
	if meta.Project != "5a339eb9-920f-4256-bbf2-d9ae6e0fb676" {
		t.Errorf("Project = %q, want 5a339eb9-920f-4256-bbf2-d9ae6e0fb676", meta.Project)
	}
	if meta.Location.ZoneID != "fr-par-1" {
		t.Errorf("ZoneID = %q, want fr-par-1", meta.Location.ZoneID)
	}
}

func TestFetchScalewayInstanceMetadata_NotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := fetchScalewayInstanceMetadata(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("fetchScalewayInstanceMetadata: want error on non-200, got nil")
	}
}

func TestZoneToRegion(t *testing.T) {
	tests := []struct {
		zone    string
		want    string
		wantErr bool
	}{
		{zone: "fr-par-1", want: "fr-par"},
		{zone: "fr-par-2", want: "fr-par"},
		{zone: "nl-ams-1", want: "nl-ams"},
		{zone: "pl-waw-3", want: "pl-waw"},
		{zone: "us-east-1", wantErr: true}, // not a supported Scaleway region
		{zone: "frpar", wantErr: true},
		{zone: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.zone, func(t *testing.T) {
			got, err := zoneToRegion(tt.zone)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("zoneToRegion(%q): want error, got %q", tt.zone, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("zoneToRegion(%q): unexpected error: %v", tt.zone, err)
			}
			if got != tt.want {
				t.Errorf("zoneToRegion(%q) = %q, want %q", tt.zone, got, tt.want)
			}
		})
	}
}
