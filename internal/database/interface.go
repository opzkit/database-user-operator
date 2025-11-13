/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package database

import "context"

// Client defines the interface for database operations
type Client interface {
	// Close closes the database connection
	Close() error

	// CreateUser creates a new database user
	CreateUser(ctx context.Context, username, password string) error

	// UserExists checks if a user exists
	UserExists(ctx context.Context, username string) (bool, error)

	// DropUser drops a database user
	DropUser(ctx context.Context, username string) error

	// CreateDatabase creates a new database
	// For PostgreSQL, owner is required; for MySQL, it's ignored
	CreateDatabase(ctx context.Context, databaseName string, owner string) error

	// DatabaseExists checks if a database exists
	DatabaseExists(ctx context.Context, databaseName string) (bool, error)

	// DropDatabase drops a database
	DropDatabase(ctx context.Context, databaseName string) error

	// GrantAllPrivileges grants all privileges on a database to a user
	GrantAllPrivileges(ctx context.Context, databaseName, username string) error

	// SetPassword sets/updates the password for a user
	SetPassword(ctx context.Context, username, password string) error

	// GetConnectionInfo returns the parsed connection information
	GetConnectionInfo() *ConnectionInfo
}
