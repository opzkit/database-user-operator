/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import (
	"strings"
	"testing"
)

func TestValidateRegion(t *testing.T) {
	tests := []struct {
		name    string
		region  string
		wantErr bool
	}{
		{
			name:    "valid us-east-1",
			region:  "us-east-1",
			wantErr: false,
		},
		{
			name:    "valid eu-west-1",
			region:  "eu-west-1",
			wantErr: false,
		},
		{
			name:    "valid ap-southeast-1",
			region:  "ap-southeast-1",
			wantErr: false,
		},
		{
			name:    "empty region - allows AWS SDK default",
			region:  "",
			wantErr: false,
		},
		{
			name:    "invalid region",
			region:  "invalid-region-1",
			wantErr: true,
		},
		{
			name:    "typo in region",
			region:  "us-east-1a",
			wantErr: true,
		},
		{
			name:    "case sensitive - uppercase",
			region:  "US-EAST-1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRegion(tt.region)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRegion(%q) error = %v, wantErr %v", tt.region, err, tt.wantErr)
			}
		})
	}
}

func TestValidAWSRegions(t *testing.T) {
	// Test that all expected regions are present
	expectedRegions := []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
		"eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1",
		"ap-northeast-1", "ap-southeast-1", "ap-south-1",
		"ca-central-1", "sa-east-1", "af-south-1",
		"me-south-1", "cn-north-1",
	}

	for _, region := range expectedRegions {
		if !ValidAWSRegions[region] {
			t.Errorf("Expected region %q to be in ValidAWSRegions", region)
		}
	}

	// Test that the map has a reasonable number of regions
	if len(ValidAWSRegions) < 30 {
		t.Errorf("ValidAWSRegions should have at least 30 regions, got %d", len(ValidAWSRegions))
	}
}

func TestDatabaseSecretToJSON(t *testing.T) {
	tests := []struct {
		name    string
		secret  *DatabaseSecret
		wantErr bool
		check   func(t *testing.T, jsonBytes []byte)
	}{
		{
			name: "postgres secret with all fields",
			secret: &DatabaseSecret{
				DBHost:      "localhost",
				DBPort:      5432,
				DBName:      "testdb",
				DBUsername:  "testuser",
				DBPassword:  "testpass",
				DatabaseURL: "postgresql://testuser:testpass@localhost:5432/testdb",
				Engine:      "postgres",
			},
			wantErr: false,
			check: func(t *testing.T, jsonBytes []byte) {
				jsonStr := string(jsonBytes)
				expectedFields := []string{
					"\"DB_HOST\":\"localhost\"",
					"\"DB_PORT\":5432",
					"\"DB_NAME\":\"testdb\"",
					"\"DB_USERNAME\":\"testuser\"",
					"\"DB_PASSWORD\":\"testpass\"",
					"\"POSTGRES_URL\":",
				}
				for _, field := range expectedFields {
					if !strings.Contains(jsonStr, field) {
						t.Errorf("Expected JSON to contain %q, got: %s", field, jsonStr)
					}
				}
			},
		},
		{
			name: "postgresql engine converts to POSTGRES_URL",
			secret: &DatabaseSecret{
				DBHost:      "localhost",
				DBPort:      5432,
				DBName:      "testdb",
				DBUsername:  "testuser",
				DBPassword:  "testpass",
				DatabaseURL: "postgresql://testuser:testpass@localhost:5432/testdb",
				Engine:      "postgresql",
			},
			wantErr: false,
			check: func(t *testing.T, jsonBytes []byte) {
				jsonStr := string(jsonBytes)
				if !strings.Contains(jsonStr, "\"POSTGRES_URL\":") {
					t.Errorf("Expected JSON to contain POSTGRES_URL for postgresql engine, got: %s", jsonStr)
				}
			},
		},
		{
			name: "mysql secret",
			secret: &DatabaseSecret{
				DBHost:      "localhost",
				DBPort:      3306,
				DBName:      "testdb",
				DBUsername:  "testuser",
				DBPassword:  "testpass",
				DatabaseURL: "mysql://testuser:testpass@localhost:3306/testdb",
				Engine:      "mysql",
			},
			wantErr: false,
			check: func(t *testing.T, jsonBytes []byte) {
				jsonStr := string(jsonBytes)
				if !strings.Contains(jsonStr, "\"MYSQL_URL\":") {
					t.Errorf("Expected JSON to contain MYSQL_URL for mysql engine, got: %s", jsonStr)
				}
			},
		},
		{
			name: "minimal secret without URL",
			secret: &DatabaseSecret{
				DBHost:     "localhost",
				DBPort:     5432,
				DBName:     "testdb",
				DBUsername: "testuser",
				DBPassword: "testpass",
			},
			wantErr: false,
			check: func(t *testing.T, jsonBytes []byte) {
				jsonStr := string(jsonBytes)
				// Should still have basic fields
				if !strings.Contains(jsonStr, "\"DB_HOST\":\"localhost\"") {
					t.Errorf("Expected JSON to contain DB_HOST, got: %s", jsonStr)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := tt.secret.ToJSON()
			if (err != nil) != tt.wantErr {
				t.Errorf("DatabaseSecret.ToJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, jsonBytes)
			}
		})
	}
}

func TestSecretErrors(t *testing.T) {
	t.Run("SecretNotFoundError", func(t *testing.T) {
		err := &SecretNotFoundError{
			SecretName: "test-secret",
			Err:        nil,
		}
		if err.Error() != "secret test-secret does not exist" {
			t.Errorf("Expected error message to contain secret name, got: %s", err.Error())
		}
	})

	t.Run("SecretMarkedForDeletionError", func(t *testing.T) {
		err := &SecretMarkedForDeletionError{
			SecretName: "test-secret",
			Err:        nil,
		}
		if err.Error() != "secret test-secret is marked for deletion" {
			t.Errorf("Expected error message to contain secret name, got: %s", err.Error())
		}
	})
}

func TestDatabaseSecretURLFieldName(t *testing.T) {
	tests := []struct {
		name      string
		engine    string
		wantField string
	}{
		{
			name:      "postgres engine",
			engine:    "postgres",
			wantField: "POSTGRES_URL",
		},
		{
			name:      "postgresql engine",
			engine:    "postgresql",
			wantField: "POSTGRES_URL",
		},
		{
			name:      "mysql engine",
			engine:    "mysql",
			wantField: "MYSQL_URL",
		},
		{
			name:      "mariadb engine",
			engine:    "mariadb",
			wantField: "MARIADB_URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := &DatabaseSecret{
				DBHost:      "localhost",
				DBPort:      5432,
				DBName:      "testdb",
				DBUsername:  "testuser",
				DBPassword:  "testpass",
				DatabaseURL: "test-url",
				Engine:      tt.engine,
			}

			jsonBytes, err := secret.ToJSON()
			if err != nil {
				t.Fatalf("ToJSON() error = %v", err)
			}

			jsonStr := string(jsonBytes)
			if !strings.Contains(jsonStr, tt.wantField) {
				t.Errorf("ToJSON() should contain field %q, got: %s", tt.wantField, jsonStr)
			}
		})
	}
}

func TestDatabaseSecretMissingFields(t *testing.T) {
	tests := []struct {
		name   string
		secret *DatabaseSecret
	}{
		{
			name: "empty username",
			secret: &DatabaseSecret{
				DBHost:     "localhost",
				DBPort:     5432,
				DBName:     "testdb",
				DBUsername: "",
				DBPassword: "testpass",
			},
		},
		{
			name: "zero port",
			secret: &DatabaseSecret{
				DBHost:     "localhost",
				DBPort:     0,
				DBName:     "testdb",
				DBUsername: "testuser",
				DBPassword: "testpass",
			},
		},
		{
			name: "minimal fields",
			secret: &DatabaseSecret{
				DBPassword: "testpass",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := tt.secret.ToJSON()
			if err != nil {
				t.Fatalf("ToJSON() error = %v", err)
			}

			// Should still produce valid JSON even with missing fields
			jsonStr := string(jsonBytes)
			if len(jsonStr) == 0 {
				t.Error("ToJSON() produced empty string")
			}
			if jsonStr[0] != '{' || jsonStr[len(jsonStr)-1] != '}' {
				t.Errorf("ToJSON() doesn't look like valid JSON: %s", jsonStr)
			}
		})
	}
}

func TestSecretErrorUnwrap(t *testing.T) {
	baseErr := &SecretNotFoundError{
		SecretName: "test-secret",
		Err:        nil,
	}

	unwrapped := baseErr.Unwrap()
	if unwrapped != nil {
		t.Errorf("Unwrap() = %v, want nil", unwrapped)
	}
}

func TestValidateRegionEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		region  string
		wantErr bool
	}{
		{
			name:    "all lowercase valid region",
			region:  "us-east-1",
			wantErr: false,
		},
		{
			name:    "region with hyphen",
			region:  "ap-southeast-1",
			wantErr: false,
		},
		{
			name:    "region with numbers",
			region:  "ap-southeast-4",
			wantErr: false,
		},
		{
			name:    "gov cloud region",
			region:  "us-gov-west-1",
			wantErr: false,
		},
		{
			name:    "china region",
			region:  "cn-north-1",
			wantErr: false,
		},
		{
			name:    "whitespace region",
			region:  " us-east-1 ",
			wantErr: true,
		},
		{
			name:    "partial match invalid",
			region:  "us-east",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRegion(tt.region)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRegion(%q) error = %v, wantErr %v", tt.region, err, tt.wantErr)
			}
		})
	}
}
