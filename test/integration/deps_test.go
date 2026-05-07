// Copyright 2025 OpzKit
//
// Licensed under the MIT License.
// See LICENSE file in the project root for full license information.

// Imported here (without the integration build tag) so `go mod tidy` keeps
// ginkgo and gomega in go.sum. Real usages live in *_test.go files behind
// //go:build integration.

package integration

import (
	_ "github.com/onsi/ginkgo/v2"
	_ "github.com/onsi/gomega"
)
