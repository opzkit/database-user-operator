/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/go-sql-driver/mysql" // MySQL driver
)

// MySQLClient provides MySQL database operations
type MySQLClient struct {
	db       *sql.DB
	connInfo *ConnectionInfo
}

// NewMySQLClient creates a new MySQL client
func NewMySQLClient(connectionString string) (*MySQLClient, error) {
	// Parse connection info first
	connInfo, err := ParseMySQLConnectionString(connectionString)
	if err != nil {
		return nil, err
	}

	// Convert to MySQL DSN format if needed
	dsn := connectionString
	if strings.HasPrefix(connectionString, "mysql://") {
		dsn, err = convertMySQLURLToDSN(connectionString)
		if err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		_ = db.Close() // Ignore error on cleanup path
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &MySQLClient{
		db:       db,
		connInfo: connInfo,
	}, nil
}

// Close closes the database connection
func (c *MySQLClient) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// CreateUser creates a new MySQL user
func (c *MySQLClient) CreateUser(ctx context.Context, username, password string) error {
	// MySQL uses CREATE USER IF NOT EXISTS
	// Create user for all hosts by default (can be restricted to specific hosts if needed)
	query := fmt.Sprintf("CREATE USER IF NOT EXISTS %s@'%%' IDENTIFIED BY %s",
		quoteMySQLIdentifier(username),
		quoteMySQLLiteral(password))

	_, err := c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// UserExists checks if a user exists
func (c *MySQLClient) UserExists(ctx context.Context, username string) (bool, error) {
	var count int
	query := "SELECT COUNT(*) FROM mysql.user WHERE user = ?"
	err := c.db.QueryRowContext(ctx, query, username).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check if user exists: %w", err)
	}
	return count > 0, nil
}

// DropUser drops a database user
func (c *MySQLClient) DropUser(ctx context.Context, username string) error {
	query := fmt.Sprintf("DROP USER IF EXISTS %s@'%%'", quoteMySQLIdentifier(username))
	_, err := c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop user: %w", err)
	}
	return nil
}

// CreateDatabase creates a new database
// The owner parameter is ignored for MySQL as it doesn't have the same ownership model as PostgreSQL
func (c *MySQLClient) CreateDatabase(ctx context.Context, databaseName string, owner string) error {
	query := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		quoteMySQLIdentifier(databaseName))
	_, err := c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	return nil
}

// DatabaseExists checks if a database exists
func (c *MySQLClient) DatabaseExists(ctx context.Context, databaseName string) (bool, error) {
	var count int
	query := "SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?"
	err := c.db.QueryRowContext(ctx, query, databaseName).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check if database exists: %w", err)
	}
	return count > 0, nil
}

// DropDatabase drops a database
func (c *MySQLClient) DropDatabase(ctx context.Context, databaseName string) error {
	query := fmt.Sprintf("DROP DATABASE IF EXISTS %s", quoteMySQLIdentifier(databaseName))
	_, err := c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop database: %w", err)
	}
	return nil
}

// GrantAllPrivileges grants all privileges on a database to a user
func (c *MySQLClient) GrantAllPrivileges(ctx context.Context, databaseName, username string) error {
	query := fmt.Sprintf("GRANT ALL PRIVILEGES ON %s.* TO %s@'%%'",
		quoteMySQLIdentifier(databaseName),
		quoteMySQLIdentifier(username))
	_, err := c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to grant privileges: %w", err)
	}

	// Flush privileges to ensure they take effect
	_, err = c.db.ExecContext(ctx, "FLUSH PRIVILEGES")
	if err != nil {
		return fmt.Errorf("failed to flush privileges: %w", err)
	}

	return nil
}

// SetPassword sets/updates the password for a user
func (c *MySQLClient) SetPassword(ctx context.Context, username, password string) error {
	query := fmt.Sprintf("ALTER USER %s@'%%' IDENTIFIED BY %s",
		quoteMySQLIdentifier(username),
		quoteMySQLLiteral(password))
	_, err := c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to set password: %w", err)
	}

	// Flush privileges to ensure password change takes effect
	_, err = c.db.ExecContext(ctx, "FLUSH PRIVILEGES")
	if err != nil {
		return fmt.Errorf("failed to flush privileges: %w", err)
	}

	return nil
}

// GetConnectionInfo returns the parsed connection information
func (c *MySQLClient) GetConnectionInfo() *ConnectionInfo {
	return c.connInfo
}

// ParseMySQLConnectionString parses a MySQL connection string
func ParseMySQLConnectionString(connectionString string) (*ConnectionInfo, error) {
	// Handle MySQL URL format: mysql://user:pass@host:port/database
	if strings.HasPrefix(connectionString, "mysql://") {
		u, err := url.Parse(connectionString)
		if err != nil {
			return nil, fmt.Errorf("failed to parse connection URL: %w", err)
		}

		info := &ConnectionInfo{
			Host:     u.Hostname(),
			Port:     u.Port(),
			Database: strings.TrimPrefix(u.Path, "/"),
		}

		if u.User != nil {
			info.Username = u.User.Username()
			info.Password, _ = u.User.Password()
		}

		if info.Port == "" {
			info.Port = "3306"
		}

		return info, nil
	}

	// Handle MySQL DSN format: user:pass@tcp(host:port)/database
	if strings.Contains(connectionString, "@tcp(") {
		return parseMySQLDSN(connectionString)
	}

	return nil, fmt.Errorf("unsupported MySQL connection string format")
}

// parseMySQLDSN parses MySQL DSN format
func parseMySQLDSN(dsn string) (*ConnectionInfo, error) {
	info := &ConnectionInfo{}

	// Extract user:password
	atIndex := strings.Index(dsn, "@")
	if atIndex > 0 {
		userPass := dsn[:atIndex]
		colonIndex := strings.Index(userPass, ":")
		if colonIndex > 0 {
			info.Username = userPass[:colonIndex]
			info.Password = userPass[colonIndex+1:]
		} else {
			info.Username = userPass
		}
	}

	// Extract host:port
	tcpStart := strings.Index(dsn, "@tcp(")
	if tcpStart >= 0 {
		tcpEnd := strings.Index(dsn[tcpStart:], ")")
		if tcpEnd > 0 {
			hostPort := dsn[tcpStart+5 : tcpStart+tcpEnd]
			colonIndex := strings.Index(hostPort, ":")
			if colonIndex > 0 {
				info.Host = hostPort[:colonIndex]
				info.Port = hostPort[colonIndex+1:]
			} else {
				info.Host = hostPort
				info.Port = "3306"
			}
		}
	}

	// Extract database
	slashIndex := strings.Index(dsn, ")/")
	if slashIndex > 0 {
		remaining := dsn[slashIndex+2:]
		questionIndex := strings.Index(remaining, "?")
		if questionIndex > 0 {
			info.Database = remaining[:questionIndex]
		} else {
			info.Database = remaining
		}
	}

	return info, nil
}

// convertMySQLURLToDSN converts MySQL URL to DSN format
func convertMySQLURLToDSN(connectionString string) (string, error) {
	info, err := ParseMySQLConnectionString(connectionString)
	if err != nil {
		return "", err
	}

	// Build DSN: user:password@tcp(host:port)/database?parseTime=true
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		info.Username,
		info.Password,
		info.Host,
		info.Port,
		info.Database)

	return dsn, nil
}

// quoteMySQLIdentifier quotes a MySQL identifier using backticks
func quoteMySQLIdentifier(name string) string {
	// Escape backticks by doubling them
	escaped := strings.ReplaceAll(name, "`", "``")
	return fmt.Sprintf("`%s`", escaped)
}

// quoteMySQLLiteral quotes a MySQL string literal using single quotes
func quoteMySQLLiteral(value string) string {
	// Escape single quotes by doubling them, and escape backslashes
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "'", "''")
	return fmt.Sprintf("'%s'", escaped)
}
