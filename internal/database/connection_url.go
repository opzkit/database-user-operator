/*
Copyright 2026 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package database

import (
	"errors"
	"net/url"
)

// errConnectionURLSyntax replaces url.Parse errors for admin connection
// strings. A *url.Error quotes the whole input, password included, and
// the controller copies reconcile errors into Database status and
// events, which anyone who can read the Database sees.
var errConnectionURLSyntax = errors.New("failed to parse connection URL: invalid URL syntax (value not shown; percent-encode special characters in the user and password)")

// parseConnectionURL parses a connection string as a URL without ever
// returning the string in an error.
func parseConnectionURL(connectionString string) (*url.URL, error) {
	u, err := url.Parse(connectionString)
	if err != nil {
		return nil, errConnectionURLSyntax
	}
	return u, nil
}
