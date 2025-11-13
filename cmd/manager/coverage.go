// Copyright 2025 OpzKit
//
// Licensed under the MIT License.
// See LICENSE file in the project root for full license information.

//go:build cover

package main

import (
	"runtime/coverage"
)

// flushCoverage writes coverage metadata and counters to the specified directory
// This function is only available when built with -cover flag
func flushCoverage(coverDir string) error {
	// Write metadata first (only needs to be done once, but it's idempotent)
	if err := coverage.WriteMetaDir(coverDir); err != nil {
		return err
	}
	// Write counters (cumulative execution counts)
	return coverage.WriteCountersDir(coverDir)
}
