/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// DatabaseUserReconciliationTotal tracks the total number of reconciliations
	DatabaseUserReconciliationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "databaseuser_reconciliation_total",
			Help: "Total number of DatabaseUser reconciliations",
		},
		[]string{"namespace", "name", "result"},
	)

	// DatabaseUserReconciliationDuration tracks the duration of reconciliations
	DatabaseUserReconciliationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "databaseuser_reconciliation_duration_seconds",
			Help:    "Duration of DatabaseUser reconciliations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"namespace", "name"},
	)

	// DatabaseUserInfo provides info about DatabaseUsers
	DatabaseUserInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "databaseuser_info",
			Help: "Information about DatabaseUser resources",
		},
		[]string{"namespace", "name", "username", "database", "phase"},
	)

	// DatabaseUserValidationErrors tracks validation errors
	DatabaseUserValidationErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "databaseuser_validation_errors_total",
			Help: "Total number of validation errors for DatabaseUser",
		},
		[]string{"namespace", "name", "error_type"},
	)

	// DatabaseUserConditions tracks the status of conditions
	DatabaseUserConditions = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "databaseuser_condition_status",
			Help: "Status of DatabaseUser conditions (1 = true, 0 = false, -1 = unknown)",
		},
		[]string{"namespace", "name", "condition"},
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		DatabaseUserReconciliationTotal,
		DatabaseUserReconciliationDuration,
		DatabaseUserInfo,
		DatabaseUserValidationErrors,
		DatabaseUserConditions,
	)
}
