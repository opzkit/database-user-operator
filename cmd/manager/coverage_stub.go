// Copyright 2025 OpzKit
//
// Licensed under the MIT License.
// See LICENSE file in the project root for full license information.

//go:build !cover

package main

// flushCoverage is a no-op when not built with coverage instrumentation
func flushCoverage(coverDir string) error {
	return nil
}
