/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package controller

import (
	"errors"
	"testing"
)

func TestIsAWSPermissionError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "AccessDeniedException",
			err:      errors.New("api error AccessDeniedException: User is not authorized to perform: secretsmanager:DescribeSecret"),
			expected: true,
		},
		{
			name:     "access denied lowercase",
			err:      errors.New("access denied to resource"),
			expected: true,
		},
		{
			name:     "not authorized",
			err:      errors.New("User is not authorized to perform this action"),
			expected: true,
		},
		{
			name:     "insufficient permissions",
			err:      errors.New("insufficient permissions to access resource"),
			expected: true,
		},
		{
			name:     "forbidden",
			err:      errors.New("403 Forbidden"),
			expected: true,
		},
		{
			name:     "UnauthorizedOperation",
			err:      errors.New("UnauthorizedOperation: You are not authorized"),
			expected: true,
		},
		{
			name:     "regular error",
			err:      errors.New("connection timeout"),
			expected: false,
		},
		{
			name:     "not found error",
			err:      errors.New("resource not found"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAWSPermissionError(tt.err)
			if result != tt.expected {
				t.Errorf("isAWSPermissionError(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}
