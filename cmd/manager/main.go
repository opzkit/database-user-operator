package main

import (
	"context"
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	databasev1alpha1 "opzkit/database-user-operator/api/v1alpha1"
	"opzkit/database-user-operator/internal/controller"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(databasev1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: 9443,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         true, // Always enabled for safe multi-replica operation
		LeaderElectionID:       "database-user-operator.opzkit.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.DatabaseReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("database-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Database")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Setup coverage collection for integration tests
	if coverDir := os.Getenv("GOCOVERDIR"); coverDir != "" {
		setupLog.Info("coverage collection enabled", "directory", coverDir)
		stopChan := make(chan struct{})
		go startCoverageFlusher(coverDir, stopChan)

		// Setup shutdown hook to flush coverage one final time and stop the flusher
		if err := mgr.Add(&coverageRunnable{coverDir: coverDir, stopChan: stopChan}); err != nil {
			setupLog.Error(err, "unable to add coverage runnable")
		}
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// coverageRunnable implements manager.Runnable to flush coverage data on shutdown
type coverageRunnable struct {
	coverDir string
	stopChan chan struct{}
}

func (c *coverageRunnable) Start(ctx context.Context) error {
	<-ctx.Done()
	setupLog.Info("manager stopping, flushing final coverage data")
	close(c.stopChan) // Stop the periodic flusher
	if err := flushCoverage(c.coverDir); err != nil {
		setupLog.Error(err, "failed to flush coverage data on shutdown")
	} else {
		setupLog.Info("successfully flushed coverage data on shutdown")
	}
	return nil
}

// startCoverageFlusher periodically writes coverage counters to disk
// This captures coverage data during long-running tests
func startCoverageFlusher(coverDir string, stopChan chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := flushCoverage(coverDir); err != nil {
				setupLog.Error(err, "failed to flush coverage data")
			} else {
				setupLog.V(1).Info("flushed coverage data")
			}
		case <-stopChan:
			return
		}
	}
}
