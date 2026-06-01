/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import "testing"

func TestResolveScalewayProjectID(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		env     string
		want    string
		wantErr bool
	}{
		{name: "spec wins", spec: "spec-proj", env: "env-proj", want: "spec-proj"},
		{name: "spec only", spec: "spec-proj", env: "", want: "spec-proj"},
		{name: "env fallback", spec: "", env: "env-proj", want: "env-proj"},
		{name: "neither set", spec: "", env: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ScalewayEnvDefaultProjectID, tt.env)
			got, err := ResolveScalewayProjectID(tt.spec)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveScalewayProjectID(%q) with env %q: want error, got %q", tt.spec, tt.env, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveScalewayProjectID(%q) with env %q: unexpected error: %v", tt.spec, tt.env, err)
			}
			if got != tt.want {
				t.Errorf("ResolveScalewayProjectID(%q) with env %q = %q, want %q", tt.spec, tt.env, got, tt.want)
			}
		})
	}
}

func TestResolveScalewayRegion(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		env     string
		want    string
		wantErr bool
	}{
		{name: "spec wins", spec: "fr-par", env: "nl-ams", want: "fr-par"},
		{name: "spec only", spec: "fr-par", env: "", want: "fr-par"},
		{name: "env fallback", spec: "", env: "nl-ams", want: "nl-ams"},
		{name: "neither set", spec: "", env: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ScalewayEnvDefaultRegion, tt.env)
			got, err := ResolveScalewayRegion(tt.spec)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveScalewayRegion(%q) with env %q: want error, got %q", tt.spec, tt.env, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveScalewayRegion(%q) with env %q: unexpected error: %v", tt.spec, tt.env, err)
			}
			if got != tt.want {
				t.Errorf("ResolveScalewayRegion(%q) with env %q = %q, want %q", tt.spec, tt.env, got, tt.want)
			}
		})
	}
}
