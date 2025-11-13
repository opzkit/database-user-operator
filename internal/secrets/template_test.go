/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDatabaseSecret_ToJSONWithTemplate(t *testing.T) {
	tests := []struct {
		name         string
		secret       *DatabaseSecret
		template     string
		wantContains []string
		wantErr      bool
		errContains  string
	}{
		{
			name: "default template (empty string)",
			secret: &DatabaseSecret{
				DBHost:      "localhost",
				DBPort:      5432,
				DBName:      "testdb",
				DBUsername:  "testuser",
				DBPassword:  "testpass",
				DatabaseURL: "postgresql://testuser:testpass@localhost:5432/testdb",
				Engine:      "postgres",
			},
			template: "",
			wantContains: []string{
				`"DB_HOST":"localhost"`,
				`"DB_PORT":5432`,
				`"DB_NAME":"testdb"`,
				`"DB_USERNAME":"testuser"`,
				`"DB_PASSWORD":"testpass"`,
				`"POSTGRES_URL":"postgresql://testuser:testpass@localhost:5432/testdb"`,
			},
		},
		{
			name: "custom template - connection string only",
			secret: &DatabaseSecret{
				DBHost:      "localhost",
				DBPort:      5432,
				DBName:      "testdb",
				DBUsername:  "testuser",
				DBPassword:  "testpass",
				DatabaseURL: "postgresql://testuser:testpass@localhost:5432/testdb",
				Engine:      "postgres",
			},
			template: `{"connectionString": "{{.DatabaseURL}}"}`,
			wantContains: []string{
				`"connectionString"`,
				`"postgresql://testuser:testpass@localhost:5432/testdb"`,
			},
		},
		{
			name: "custom template - separate fields with custom names",
			secret: &DatabaseSecret{
				DBHost:      "db.example.com",
				DBPort:      3306,
				DBName:      "myapp",
				DBUsername:  "app_user",
				DBPassword:  "secret123",
				DatabaseURL: "mysql://app_user:secret123@db.example.com:3306/myapp",
				Engine:      "mysql",
			},
			template: `{
				"host": "{{.DBHost}}",
				"port": {{.DBPort}},
				"database": "{{.DBName}}",
				"user": "{{.DBUsername}}",
				"password": "{{.DBPassword}}"
			}`,
			wantContains: []string{
				`"host":"db.example.com"`,
				`"port":3306`,
				`"database":"myapp"`,
				`"user":"app_user"`,
				`"password":"secret123"`,
			},
		},
		{
			name: "custom template - spring boot style",
			secret: &DatabaseSecret{
				DBHost:      "localhost",
				DBPort:      5432,
				DBName:      "testdb",
				DBUsername:  "testuser",
				DBPassword:  "testpass",
				DatabaseURL: "postgresql://testuser:testpass@localhost:5432/testdb",
				Engine:      "postgres",
			},
			template: `{
				"spring.datasource.url": "jdbc:postgresql://{{.DBHost}}:{{.DBPort}}/{{.DBName}}",
				"spring.datasource.username": "{{.DBUsername}}",
				"spring.datasource.password": "{{.DBPassword}}"
			}`,
			wantContains: []string{
				`"spring.datasource.url":"jdbc:postgresql://localhost:5432/testdb"`,
				`"spring.datasource.username":"testuser"`,
				`"spring.datasource.password":"testpass"`,
			},
		},
		{
			name: "custom template - environment variables style",
			secret: &DatabaseSecret{
				DBHost:     "localhost",
				DBPort:     5432,
				DBName:     "testdb",
				DBUsername: "testuser",
				DBPassword: "testpass",
				Engine:     "postgres",
			},
			template: `{
				"DATABASE_HOST": "{{.DBHost}}",
				"DATABASE_PORT": "{{.DBPort}}",
				"DATABASE_NAME": "{{.DBName}}",
				"DATABASE_USER": "{{.DBUsername}}",
				"DATABASE_PASSWORD": "{{.DBPassword}}"
			}`,
			wantContains: []string{
				`"DATABASE_HOST":"localhost"`,
				`"DATABASE_PORT":"5432"`,
				`"DATABASE_NAME":"testdb"`,
				`"DATABASE_USER":"testuser"`,
				`"DATABASE_PASSWORD":"testpass"`,
			},
		},
		{
			name: "invalid template - produces invalid JSON",
			secret: &DatabaseSecret{
				DBHost:     "localhost",
				DBPort:     5432,
				DBUsername: "testuser",
				DBPassword: "testpass",
			},
			template:    `{"invalid": {{.DBHost}}}`, // Invalid JSON - extra brace
			wantErr:     true,
			errContains: "not valid JSON",
		},
		{
			name: "invalid template - parse error",
			secret: &DatabaseSecret{
				DBHost:     "localhost",
				DBPort:     5432,
				DBUsername: "testuser",
				DBPassword: "testpass",
			},
			template:    `{"field": "{{.DBHost"}`, // Unclosed template
			wantErr:     true,
			errContains: "failed to parse secret template",
		},
		{
			name: "invalid template - undefined field",
			secret: &DatabaseSecret{
				DBHost:     "localhost",
				DBPort:     5432,
				DBUsername: "testuser",
				DBPassword: "testpass",
			},
			template:    `{"field": "{{.UndefinedField}}"}`,
			wantErr:     true,
			errContains: "failed to execute secret template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.secret.ToJSONWithTemplate(tt.template)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToJSONWithTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if err != nil && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ToJSONWithTemplate() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			// Verify it's valid JSON
			var jsonCheck map[string]interface{}
			if err := json.Unmarshal(got, &jsonCheck); err != nil {
				t.Errorf("ToJSONWithTemplate() produced invalid JSON: %v\nOutput: %s", err, string(got))
				return
			}

			// Re-marshal to remove formatting differences
			compactJSON, err := json.Marshal(jsonCheck)
			if err != nil {
				t.Errorf("Failed to re-marshal JSON: %v", err)
				return
			}

			// Check for expected content in compacted version
			gotStr := string(compactJSON)
			for _, want := range tt.wantContains {
				if !strings.Contains(gotStr, want) {
					t.Errorf("ToJSONWithTemplate() output doesn't contain %q\nGot: %s", want, gotStr)
				}
			}
		})
	}
}

func TestDatabaseSecret_ToJSON_UsesDefaultTemplate(t *testing.T) {
	secret := &DatabaseSecret{
		DBHost:      "localhost",
		DBPort:      5432,
		DBName:      "testdb",
		DBUsername:  "testuser",
		DBPassword:  "testpass",
		DatabaseURL: "postgresql://testuser:testpass@localhost:5432/testdb",
		Engine:      "postgres",
	}

	// ToJSON should use default template
	jsonDefault, err := secret.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	// ToJSONWithTemplate with empty string should produce same result
	jsonWithEmptyTemplate, err := secret.ToJSONWithTemplate("")
	if err != nil {
		t.Fatalf("ToJSONWithTemplate(\"\") error = %v", err)
	}

	if string(jsonDefault) != string(jsonWithEmptyTemplate) {
		t.Errorf("ToJSON() and ToJSONWithTemplate(\"\") produced different results:\nToJSON: %s\nToJSONWithTemplate: %s",
			string(jsonDefault), string(jsonWithEmptyTemplate))
	}
}
