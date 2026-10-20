/*
Copyright 2026 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package database

import (
	"strings"
	"testing"
)

// Parse errors reach Database status and events; they must never carry
// any part of the connection string.
func TestParseErrorsOmitConnectionString(t *testing.T) {
	const secret = "s3cr%zzt"
	tests := []struct {
		name    string
		connStr string
		parse   func(string) (*ConnectionInfo, error)
	}{
		{"postgres invalid escape", "postgres://admin:" + secret + "@db:5432/postgres", ParseConnectionString},
		{"postgres invalid port", "postgres://admin:" + secret + "@db:port/postgres", ParseConnectionString},
		{"mysql invalid escape", "mysql://admin:" + secret + "@db:3306/mysql", ParseMySQLConnectionString},
		{"mysql invalid port", "mysql://admin:" + secret + "@db:port/mysql", ParseMySQLConnectionString},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.parse(tt.connStr)
			if err == nil {
				t.Fatal("expected a parse error")
			}
			for _, fragment := range []string{"s3cr", "%zz", "admin", "db:"} {
				if strings.Contains(err.Error(), fragment) {
					t.Errorf("error %q contains %q from the connection string", err, fragment)
				}
			}
		})
	}
}
