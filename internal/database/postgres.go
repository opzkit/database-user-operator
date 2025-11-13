/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgresClient provides PostgreSQL database operations
type PostgresClient struct {
	db       *sql.DB
	connInfo *ConnectionInfo
}

// ConnectionInfo contains parsed connection information
type ConnectionInfo struct {
	Host     string
	Port     string
	Database string
	Username string
	Password string
	SSLMode  string
}

// NewPostgresClient creates a new PostgreSQL client
func NewPostgresClient(connectionString string) (*PostgresClient, error) {
	// Parse connection info first
	connInfo, err := ParseConnectionString(connectionString)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		_ = db.Close() // Ignore error on cleanup path
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgresClient{
		db:       db,
		connInfo: connInfo,
	}, nil
}

// Close closes the database connection
func (c *PostgresClient) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// ParseConnectionString parses a PostgreSQL connection string
func ParseConnectionString(connectionString string) (*ConnectionInfo, error) {
	// Handle both URL and DSN formats
	if strings.HasPrefix(connectionString, "postgres://") || strings.HasPrefix(connectionString, "postgresql://") {
		u, err := url.Parse(connectionString)
		if err != nil {
			return nil, fmt.Errorf("failed to parse connection URL: %w", err)
		}

		info := &ConnectionInfo{
			Host:     u.Hostname(),
			Port:     u.Port(),
			Database: strings.TrimPrefix(u.Path, "/"),
			SSLMode:  u.Query().Get("sslmode"),
		}

		if u.User != nil {
			info.Username = u.User.Username()
			info.Password, _ = u.User.Password()
		}

		if info.Port == "" {
			info.Port = "5432"
		}
		if info.SSLMode == "" {
			info.SSLMode = "require"
		}

		return info, nil
	}

	return nil, fmt.Errorf("unsupported connection string format, expected postgres:// or postgresql:// URL")
}

// DatabaseExists checks if a database exists
func (c *PostgresClient) DatabaseExists(ctx context.Context, dbName string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`
	var exists bool
	err := c.db.QueryRowContext(ctx, query, dbName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if database exists: %w", err)
	}
	return exists, nil
}

// CreateDatabase creates a new database
func (c *PostgresClient) CreateDatabase(ctx context.Context, dbName string, owner string) error {
	// Check if database already exists
	exists, err := c.DatabaseExists(ctx, dbName)
	if err != nil {
		return err
	}
	if exists {
		return nil // Database already exists, nothing to do
	}

	// Create database
	query := fmt.Sprintf("CREATE DATABASE %s OWNER %s", quoteIdentifier(dbName), quoteIdentifier(owner))
	_, err = c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	// Add comment to database indicating it's managed by the operator
	commentQuery := fmt.Sprintf("COMMENT ON DATABASE %s IS %s",
		quoteIdentifier(dbName),
		quoteLiteral("Managed by database-user-operator"))
	_, err = c.db.ExecContext(ctx, commentQuery)
	if err != nil {
		return fmt.Errorf("failed to add comment to database: %w", err)
	}

	return nil
}

// DropDatabase drops a database
func (c *PostgresClient) DropDatabase(ctx context.Context, dbName string) error {
	// Terminate existing connections to the database
	terminateQuery := `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1 AND pid <> pg_backend_pid()
	`
	_, err := c.db.ExecContext(ctx, terminateQuery, dbName)
	if err != nil {
		return fmt.Errorf("failed to terminate connections: %w", err)
	}

	// Drop database
	query := fmt.Sprintf("DROP DATABASE IF EXISTS %s", quoteIdentifier(dbName))
	_, err = c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop database: %w", err)
	}

	return nil
}

// UserExists checks if a user exists
func (c *PostgresClient) UserExists(ctx context.Context, username string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)`
	var exists bool
	err := c.db.QueryRowContext(ctx, query, username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if user exists: %w", err)
	}
	return exists, nil
}

// CreateUser creates a new user with a password
func (c *PostgresClient) CreateUser(ctx context.Context, username, password string) error {
	// Check if user already exists
	exists, err := c.UserExists(ctx, username)
	if err != nil {
		return err
	}

	if exists {
		// User exists, update password
		query := fmt.Sprintf("ALTER USER %s WITH PASSWORD %s", quoteIdentifier(username), quoteLiteral(password))
		_, err = c.db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to update user password: %w", err)
		}
	} else {
		// Create new user
		query := fmt.Sprintf("CREATE USER %s WITH PASSWORD %s", quoteIdentifier(username), quoteLiteral(password))
		_, err = c.db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}
	}

	// Add comment to role indicating it's managed by the operator
	commentQuery := fmt.Sprintf("COMMENT ON ROLE %s IS %s",
		quoteIdentifier(username),
		quoteLiteral("Managed by database-user-operator"))
	_, err = c.db.ExecContext(ctx, commentQuery)
	if err != nil {
		return fmt.Errorf("failed to add comment to role: %w", err)
	}

	return nil
}

// DropUser drops a user
func (c *PostgresClient) DropUser(ctx context.Context, username string) error {
	// Revoke all privileges first
	revokeQuery := fmt.Sprintf("REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM %s", quoteIdentifier(username))
	_, _ = c.db.ExecContext(ctx, revokeQuery)

	// Drop user
	query := fmt.Sprintf("DROP USER IF EXISTS %s", quoteIdentifier(username))
	_, err := c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop user: %w", err)
	}

	return nil
}

// GrantPrivileges grants privileges to a user on a database
func (c *PostgresClient) GrantPrivileges(ctx context.Context, username, dbName string, privileges []string) error {
	// Connect to the target database to grant privileges
	connInfo, err := c.getConnectionInfo()
	if err != nil {
		return err
	}

	// Create connection string for the target database
	targetConnStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		url.QueryEscape(connInfo.Username),
		url.QueryEscape(connInfo.Password),
		connInfo.Host,
		connInfo.Port,
		dbName,
		connInfo.SSLMode,
	)

	targetDB, err := sql.Open("postgres", targetConnStr)
	if err != nil {
		return fmt.Errorf("failed to connect to target database: %w", err)
	}
	defer func() {
		_ = targetDB.Close() // Ignore error on cleanup
	}()

	if err := targetDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping target database: %w", err)
	}

	// Build privilege string
	privStr := strings.Join(privileges, ", ")

	// Grant database-level privileges
	query := fmt.Sprintf("GRANT %s ON DATABASE %s TO %s", privStr, quoteIdentifier(dbName), quoteIdentifier(username))
	_, err = c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to grant database privileges: %w", err)
	}

	// Grant schema-level privileges
	schemaQuery := fmt.Sprintf("GRANT ALL ON SCHEMA public TO %s", quoteIdentifier(username))
	_, err = targetDB.ExecContext(ctx, schemaQuery)
	if err != nil {
		return fmt.Errorf("failed to grant schema privileges: %w", err)
	}

	// Grant table privileges (for existing and future tables)
	tableQuery := fmt.Sprintf("GRANT ALL ON ALL TABLES IN SCHEMA public TO %s", quoteIdentifier(username))
	_, err = targetDB.ExecContext(ctx, tableQuery)
	if err != nil {
		return fmt.Errorf("failed to grant table privileges: %w", err)
	}

	// Grant default privileges for future tables
	defaultQuery := fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO %s", quoteIdentifier(username))
	_, err = targetDB.ExecContext(ctx, defaultQuery)
	if err != nil {
		return fmt.Errorf("failed to grant default privileges: %w", err)
	}

	return nil
}

// GrantAllPrivileges grants all privileges on a database to a user
func (c *PostgresClient) GrantAllPrivileges(ctx context.Context, databaseName, username string) error {
	return c.GrantPrivileges(ctx, username, databaseName, []string{"ALL"})
}

// SetPassword sets/updates the password for a user
func (c *PostgresClient) SetPassword(ctx context.Context, username, password string) error {
	query := fmt.Sprintf("ALTER USER %s WITH PASSWORD %s", quoteIdentifier(username), quoteLiteral(password))
	_, err := c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to set password: %w", err)
	}
	return nil
}

// GeneratePassword generates a secure random password
func GeneratePassword(length int) (string, error) {
	if length < 16 {
		length = 32 // Default to 32 characters for security
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random password: %w", err)
	}

	// Use base64 encoding for a mix of characters
	password := base64.URLEncoding.EncodeToString(bytes)
	// Trim to requested length
	if len(password) > length {
		password = password[:length]
	}

	return password, nil
}

// getConnectionInfo returns the stored connection info
func (c *PostgresClient) getConnectionInfo() (*ConnectionInfo, error) {
	if c.connInfo == nil {
		return nil, fmt.Errorf("connection info not available")
	}
	return c.connInfo, nil
}

// GetConnectionInfo returns the stored connection info (public method)
func (c *PostgresClient) GetConnectionInfo() *ConnectionInfo {
	return c.connInfo
}

// quoteIdentifier quotes an identifier (table name, column name, etc.)
func quoteIdentifier(name string) string {
	return fmt.Sprintf(`"%s"`, strings.ReplaceAll(name, `"`, `""`))
}

// quoteLiteral quotes a literal value (string, password, etc.)
func quoteLiteral(value string) string {
	escaped := strings.ReplaceAll(value, "'", "''")
	return fmt.Sprintf("'%s'", escaped)
}
