// Copyright 2025 OpzKit
//
// Licensed under the MIT License.
// See LICENSE file in the project root for full license information.

//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	databasev1alpha1 "opzkit/database-user-operator/api/v1alpha1"
)

const (
	timeout  = time.Minute * 5
	interval = time.Second * 5
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	ctx       context.Context
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Test Suite")
}

var _ = BeforeSuite(func() {
	By("Setting up integration test environment")
	ctx = context.Background()

	// Get kubeconfig
	By("Getting kubeconfig")
	var err error
	cfg, err = config.GetConfig()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// Register our API types
	By("Registering API types")
	err = databasev1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create k8s client
	By("Creating Kubernetes client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Verify databases namespace exists
	By("Verifying databases namespace exists")
	ns := &corev1.Namespace{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: "databases"}, ns)
	Expect(err).NotTo(HaveOccurred())

	// Verify operator is running
	By("Waiting for operator pods to be running in db-system namespace")
	Eventually(func() error {
		pods := &corev1.PodList{}
		err := k8sClient.List(ctx, pods, client.InNamespace("db-system"))
		if err != nil {
			return err
		}
		if len(pods.Items) == 0 {
			return fmt.Errorf("no operator pods found in db-system namespace")
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				return fmt.Errorf("operator pod %s not running: %s", pod.Name, pod.Status.Phase)
			}
		}
		return nil
	}, timeout, interval).Should(Succeed())

	By("Integration test environment ready")
})

// Helper functions

func createDatabase(namespace, name string, spec databasev1alpha1.DatabaseSpec) *databasev1alpha1.Database {
	database := &databasev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}
	Expect(k8sClient.Create(ctx, database)).Should(Succeed())
	return database
}

func getDatabase(namespace, name string) (*databasev1alpha1.Database, error) {
	db := &databasev1alpha1.Database{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, db)
	return db, err
}

func deleteDatabase(namespace, name string) error {
	db := &databasev1alpha1.Database{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, db)
	if err != nil {
		return err
	}
	return k8sClient.Delete(ctx, db)
}

func waitForDatabasePhase(namespace, name, expectedPhase string) {
	Eventually(func() string {
		db, err := getDatabase(namespace, name)
		if err != nil {
			return ""
		}
		return db.Status.Phase
	}, timeout, interval).Should(Equal(expectedPhase))
}

func waitForDatabaseCreated(namespace, name string) {
	Eventually(func() bool {
		db, err := getDatabase(namespace, name)
		if err != nil {
			return false
		}
		return db.Status.DatabaseCreated && db.Status.UserCreated
	}, timeout, interval).Should(BeTrue())
}

func waitForDatabaseDeleted(namespace, name string) {
	Eventually(func() bool {
		_, err := getDatabase(namespace, name)
		return err != nil
	}, timeout, interval).Should(BeTrue())
}

func randomString(length int) string {
	return fmt.Sprintf("%d", time.Now().UnixNano())[:length]
}
