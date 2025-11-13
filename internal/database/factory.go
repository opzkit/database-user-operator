/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package database

import (
	"fmt"
	"strings"
)

// NewClient creates a new database client based on the engine type
func NewClient(engine, connectionString string) (Client, error) {
	// Normalize engine name
	normalizedEngine := strings.ToLower(engine)

	switch normalizedEngine {
	case "postgres", "postgresql":
		return NewPostgresClient(connectionString)
	case "mysql", "mariadb":
		return NewMySQLClient(connectionString)
	default:
		return nil, fmt.Errorf("unsupported database engine: %s", engine)
	}
}
