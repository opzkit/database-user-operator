/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package database

import (
	"testing"
)

func TestParseConnectionString(t *testing.T) {
	tests := []struct {
		name          string
		connectionStr string
		wantHost      string
		wantPort      string
		wantDatabase  string
		wantUsername  string
		wantSSLMode   string
		wantErr       bool
	}{
		{
			name:          "full postgres URL",
			connectionStr: "postgres://user:pass@localhost:5432/mydb?sslmode=require",
			wantHost:      "localhost",
			wantPort:      "5432",
			wantDatabase:  "mydb",
			wantUsername:  "user",
			wantSSLMode:   "require",
			wantErr:       false,
		},
		{
			name:          "postgresql URL",
			connectionStr: "postgresql://admin:password@db.example.com:5432/proddb?sslmode=verify-full",
			wantHost:      "db.example.com",
			wantPort:      "5432",
			wantDatabase:  "proddb",
			wantUsername:  "admin",
			wantSSLMode:   "verify-full",
			wantErr:       false,
		},
		{
			name:          "URL without port - should default to 5432",
			connectionStr: "postgres://user:pass@localhost/mydb",
			wantHost:      "localhost",
			wantPort:      "5432",
			wantDatabase:  "mydb",
			wantUsername:  "user",
			wantSSLMode:   "require",
			wantErr:       false,
		},
		{
			name:          "URL without sslmode - should default to require",
			connectionStr: "postgres://user:pass@localhost:5432/mydb",
			wantHost:      "localhost",
			wantPort:      "5432",
			wantDatabase:  "mydb",
			wantUsername:  "user",
			wantSSLMode:   "require",
			wantErr:       false,
		},
		{
			name:          "URL with special characters in password",
			connectionStr: "postgres://user:p@ss%40word@localhost:5432/mydb",
			wantHost:      "localhost",
			wantPort:      "5432",
			wantDatabase:  "mydb",
			wantUsername:  "user",
			wantErr:       false,
		},
		{
			name:          "URL with IPv4 host",
			connectionStr: "postgres://user:pass@192.168.1.100:5432/mydb",
			wantHost:      "192.168.1.100",
			wantPort:      "5432",
			wantDatabase:  "mydb",
			wantUsername:  "user",
			wantErr:       false,
		},
		{
			name:          "invalid scheme",
			connectionStr: "mysql://user:pass@localhost:3306/mydb",
			wantErr:       true,
		},
		{
			name:          "not a URL",
			connectionStr: "host=localhost port=5432 dbname=mydb user=user password=pass",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConnectionString(tt.connectionStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConnectionString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if got.Host != tt.wantHost {
				t.Errorf("ParseConnectionString() Host = %v, want %v", got.Host, tt.wantHost)
			}
			if got.Port != tt.wantPort {
				t.Errorf("ParseConnectionString() Port = %v, want %v", got.Port, tt.wantPort)
			}
			if got.Database != tt.wantDatabase {
				t.Errorf("ParseConnectionString() Database = %v, want %v", got.Database, tt.wantDatabase)
			}
			if got.Username != tt.wantUsername {
				t.Errorf("ParseConnectionString() Username = %v, want %v", got.Username, tt.wantUsername)
			}
			if tt.wantSSLMode != "" && got.SSLMode != tt.wantSSLMode {
				t.Errorf("ParseConnectionString() SSLMode = %v, want %v", got.SSLMode, tt.wantSSLMode)
			}
		})
	}
}

func TestGeneratePassword(t *testing.T) {
	tests := []struct {
		name       string
		length     int
		wantLength int
	}{
		{
			name:       "short length defaults to 32",
			length:     8,
			wantLength: 32, // Function enforces minimum of 32 for security
		},
		{
			name:       "16 characters",
			length:     16,
			wantLength: 16,
		},
		{
			name:       "32 characters",
			length:     32,
			wantLength: 32,
		},
		{
			name:       "64 characters",
			length:     64,
			wantLength: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GeneratePassword(tt.length)
			if err != nil {
				t.Errorf("GeneratePassword() error = %v", err)
				return
			}

			if len(got) != tt.wantLength {
				t.Errorf("GeneratePassword() length = %v, want %v", len(got), tt.wantLength)
			}

			// Verify it's not empty
			if got == "" {
				t.Error("GeneratePassword() returned empty string")
			}

			// Verify two passwords are different
			got2, err := GeneratePassword(tt.length)
			if err != nil {
				t.Errorf("GeneratePassword() second call error = %v", err)
				return
			}

			if got == got2 {
				t.Error("GeneratePassword() returned identical passwords - should be random")
			}
		})
	}
}

func TestQuoteIdentifier(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple identifier",
			input: "users",
			want:  `"users"`,
		},
		{
			name:  "identifier with underscore",
			input: "user_accounts",
			want:  `"user_accounts"`,
		},
		{
			name:  "identifier with double quote",
			input: `table"name`,
			want:  `"table""name"`,
		},
		{
			name:  "mixed case",
			input: "MyTable",
			want:  `"MyTable"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quoteIdentifier(tt.input)
			if got != tt.want {
				t.Errorf("quoteIdentifier() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQuoteLiteral(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple string",
			input: "hello",
			want:  "'hello'",
		},
		{
			name:  "string with single quote",
			input: "it's",
			want:  "'it''s'",
		},
		{
			name:  "string with multiple single quotes",
			input: "it's a 'test'",
			want:  "'it''s a ''test'''",
		},
		{
			name:  "password with special characters",
			input: "p@ss'word!",
			want:  "'p@ss''word!'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quoteLiteral(tt.input)
			if got != tt.want {
				t.Errorf("quoteLiteral() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseConnectionStringEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		connStr string
		wantErr bool
	}{
		{
			name:    "URL with no database path",
			connStr: "postgres://user:pass@localhost:5432",
			wantErr: false,
		},
		{
			name:    "URL with query parameters",
			connStr: "postgres://user:pass@localhost:5432/db?sslmode=disable&connect_timeout=10",
			wantErr: false,
		},
		{
			name:    "URL with @ in password",
			connStr: "postgres://user:p@ssw@rd@localhost:5432/db",
			wantErr: false,
		},
		{
			name:    "http URL should fail",
			connStr: "http://localhost:8080",
			wantErr: true,
		},
		{
			name:    "empty string should fail",
			connStr: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConnectionString(tt.connStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConnectionString() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConnectionInfoDefaults(t *testing.T) {
	connStr := "postgres://user:pass@localhost/mydb"
	info, err := ParseConnectionString(connStr)
	if err != nil {
		t.Fatalf("ParseConnectionString() error = %v", err)
	}

	// Check defaults
	if info.Port != "5432" {
		t.Errorf("Default port = %v, want 5432", info.Port)
	}
	if info.SSLMode != "require" {
		t.Errorf("Default SSLMode = %v, want require", info.SSLMode)
	}
}

func TestGeneratePasswordRandomness(t *testing.T) {
	// Generate multiple passwords and ensure they're different
	passwords := make(map[string]bool)
	count := 100

	for i := 0; i < count; i++ {
		password, err := GeneratePassword(32)
		if err != nil {
			t.Fatalf("GeneratePassword() error = %v", err)
		}
		if passwords[password] {
			t.Errorf("GeneratePassword() produced duplicate password: %s", password)
		}
		passwords[password] = true
	}

	if len(passwords) != count {
		t.Errorf("Generated %d unique passwords, want %d", len(passwords), count)
	}
}

func TestGeneratePasswordLength(t *testing.T) {
	tests := []struct {
		name       string
		length     int
		wantMinLen int
	}{
		{
			name:       "very small length uses minimum",
			length:     1,
			wantMinLen: 32,
		},
		{
			name:       "below minimum uses minimum",
			length:     10,
			wantMinLen: 32,
		},
		{
			name:       "at minimum",
			length:     32,
			wantMinLen: 32,
		},
		{
			name:       "above minimum",
			length:     64,
			wantMinLen: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			password, err := GeneratePassword(tt.length)
			if err != nil {
				t.Fatalf("GeneratePassword() error = %v", err)
			}

			if len(password) < tt.wantMinLen {
				t.Errorf("password length = %d, want at least %d", len(password), tt.wantMinLen)
			}
		})
	}
}

func TestQuoteIdentifierSQLInjection(t *testing.T) {
	// Test that SQL injection attempts are properly quoted
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "SQL injection attempt 1",
			input: "users; DROP TABLE users;--",
		},
		{
			name:  "SQL injection attempt 2",
			input: "1' OR '1'='1",
		},
		{
			name:  "Double quote escape",
			input: `table"; DROP TABLE users;--`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quoted := quoteIdentifier(tt.input)

			// Should be wrapped in double quotes
			if quoted[0] != '"' || quoted[len(quoted)-1] != '"' {
				t.Errorf("quoteIdentifier() = %v, should be wrapped in double quotes", quoted)
			}

			// Should not contain unescaped double quotes in the middle
			inner := quoted[1 : len(quoted)-1]
			for i := 0; i < len(inner); i++ {
				if inner[i] == '"' {
					// Next char should also be a quote (escaped)
					if i+1 >= len(inner) || inner[i+1] != '"' {
						t.Errorf("quoteIdentifier() has unescaped quote at position %d: %v", i, quoted)
					}
					i++ // Skip the escaped quote
				}
			}
		})
	}
}

func TestQuoteLiteralSQLInjection(t *testing.T) {
	// Test that SQL injection attempts in literals are properly escaped
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "SQL injection in password",
			input: "'; DROP TABLE users;--",
		},
		{
			name:  "OR 1=1 injection",
			input: "' OR '1'='1",
		},
		{
			name:  "Multiple quotes",
			input: "'''''''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quoted := quoteLiteral(tt.input)

			// Should be wrapped in single quotes
			if quoted[0] != '\'' || quoted[len(quoted)-1] != '\'' {
				t.Errorf("quoteLiteral() = %v, should be wrapped in single quotes", quoted)
			}

			// Count single quotes - each input quote should be doubled
			inputQuotes := 0
			for _, c := range tt.input {
				if c == '\'' {
					inputQuotes++
				}
			}

			outputQuotes := 0
			for _, c := range quoted {
				if c == '\'' {
					outputQuotes++
				}
			}

			// Should have 2 wrapping quotes + 2x input quotes (escaped)
			expectedQuotes := 2 + (inputQuotes * 2)
			if outputQuotes != expectedQuotes {
				t.Errorf("quoteLiteral() has %d quotes, want %d (input had %d quotes)", outputQuotes, expectedQuotes, inputQuotes)
			}
		})
	}
}
