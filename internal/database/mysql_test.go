/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package database

import (
	"testing"
)

func TestParseMySQLConnectionString(t *testing.T) {
	tests := []struct {
		name          string
		connectionStr string
		wantHost      string
		wantPort      string
		wantDatabase  string
		wantUsername  string
		wantErr       bool
	}{
		{
			name:          "full MySQL URL",
			connectionStr: "mysql://user:pass@localhost:3306/mydb",
			wantHost:      "localhost",
			wantPort:      "3306",
			wantDatabase:  "mydb",
			wantUsername:  "user",
			wantErr:       false,
		},
		{
			name:          "MySQL URL without port - should default to 3306",
			connectionStr: "mysql://user:pass@localhost/mydb",
			wantHost:      "localhost",
			wantPort:      "3306",
			wantDatabase:  "mydb",
			wantUsername:  "user",
			wantErr:       false,
		},
		{
			name:          "MySQL DSN format",
			connectionStr: "user:pass@tcp(localhost:3306)/mydb",
			wantHost:      "localhost",
			wantPort:      "3306",
			wantDatabase:  "mydb",
			wantUsername:  "user",
			wantErr:       false,
		},
		{
			name:          "MySQL DSN with IP address",
			connectionStr: "user:pass@tcp(192.168.1.100:3306)/mydb",
			wantHost:      "192.168.1.100",
			wantPort:      "3306",
			wantDatabase:  "mydb",
			wantUsername:  "user",
			wantErr:       false,
		},
		{
			name:          "MySQL URL with special characters in password",
			connectionStr: "mysql://user:p@ss%40word@localhost:3306/mydb",
			wantHost:      "localhost",
			wantPort:      "3306",
			wantDatabase:  "mydb",
			wantUsername:  "user",
			wantErr:       false,
		},
		{
			name:          "invalid scheme",
			connectionStr: "postgres://user:pass@localhost:5432/mydb",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMySQLConnectionString(tt.connectionStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMySQLConnectionString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if got.Host != tt.wantHost {
				t.Errorf("ParseMySQLConnectionString() Host = %v, want %v", got.Host, tt.wantHost)
			}
			if got.Port != tt.wantPort {
				t.Errorf("ParseMySQLConnectionString() Port = %v, want %v", got.Port, tt.wantPort)
			}
			if got.Database != tt.wantDatabase {
				t.Errorf("ParseMySQLConnectionString() Database = %v, want %v", got.Database, tt.wantDatabase)
			}
			if got.Username != tt.wantUsername {
				t.Errorf("ParseMySQLConnectionString() Username = %v, want %v", got.Username, tt.wantUsername)
			}
		})
	}
}

func TestConvertMySQLURLToDSN(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name:    "basic URL conversion",
			url:     "mysql://user:pass@localhost:3306/mydb",
			want:    "user:pass@tcp(localhost:3306)/mydb?parseTime=true",
			wantErr: false,
		},
		{
			name:    "URL with default port",
			url:     "mysql://user:pass@localhost/mydb",
			want:    "user:pass@tcp(localhost:3306)/mydb?parseTime=true",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertMySQLURLToDSN(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertMySQLURLToDSN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("convertMySQLURLToDSN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseMySQLDSN(t *testing.T) {
	tests := []struct {
		name         string
		dsn          string
		wantHost     string
		wantPort     string
		wantDatabase string
		wantUsername string
	}{
		{
			name:         "basic DSN",
			dsn:          "user:pass@tcp(localhost:3306)/mydb",
			wantHost:     "localhost",
			wantPort:     "3306",
			wantDatabase: "mydb",
			wantUsername: "user",
		},
		{
			name:         "DSN with parameters",
			dsn:          "user:pass@tcp(localhost:3306)/mydb?parseTime=true",
			wantHost:     "localhost",
			wantPort:     "3306",
			wantDatabase: "mydb",
			wantUsername: "user",
		},
		{
			name:         "DSN without port",
			dsn:          "user:pass@tcp(localhost)/mydb",
			wantHost:     "localhost",
			wantPort:     "3306",
			wantDatabase: "mydb",
			wantUsername: "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMySQLDSN(tt.dsn)
			if err != nil {
				t.Errorf("parseMySQLDSN() error = %v", err)
				return
			}

			if got.Host != tt.wantHost {
				t.Errorf("parseMySQLDSN() Host = %v, want %v", got.Host, tt.wantHost)
			}
			if got.Port != tt.wantPort {
				t.Errorf("parseMySQLDSN() Port = %v, want %v", got.Port, tt.wantPort)
			}
			if got.Database != tt.wantDatabase {
				t.Errorf("parseMySQLDSN() Database = %v, want %v", got.Database, tt.wantDatabase)
			}
			if got.Username != tt.wantUsername {
				t.Errorf("parseMySQLDSN() Username = %v, want %v", got.Username, tt.wantUsername)
			}
		})
	}
}

func TestQuoteMySQLIdentifier(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple identifier",
			input: "users",
			want:  "`users`",
		},
		{
			name:  "identifier with underscore",
			input: "user_accounts",
			want:  "`user_accounts`",
		},
		{
			name:  "identifier with backtick",
			input: "table`name",
			want:  "`table``name`",
		},
		{
			name:  "identifier with multiple backticks",
			input: "tab`le`name",
			want:  "`tab``le``name`",
		},
		{
			name:  "mixed case",
			input: "MyTable",
			want:  "`MyTable`",
		},
		{
			name:  "SQL injection attempt",
			input: "users; DROP TABLE users;--",
			want:  "`users; DROP TABLE users;--`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quoteMySQLIdentifier(tt.input)
			if got != tt.want {
				t.Errorf("quoteMySQLIdentifier() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQuoteMySQLLiteral(t *testing.T) {
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
			name:  "string with backslash",
			input: "path\\to\\file",
			want:  "'path\\\\to\\\\file'",
		},
		{
			name:  "string with backslash and quote",
			input: "path\\to\\'file'",
			want:  "'path\\\\to\\\\''file'''",
		},
		{
			name:  "password with special characters",
			input: "p@ss'word!",
			want:  "'p@ss''word!'",
		},
		{
			name:  "SQL injection attempt",
			input: "'; DROP TABLE users;--",
			want:  "'''; DROP TABLE users;--'",
		},
		{
			name:  "OR 1=1 injection",
			input: "' OR '1'='1",
			want:  "''' OR ''1''=''1'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quoteMySQLLiteral(tt.input)
			if got != tt.want {
				t.Errorf("quoteMySQLLiteral() = %v, want %v", got, tt.want)
			}
		})
	}
}
