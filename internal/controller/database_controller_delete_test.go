/*
Copyright 2026 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package controller

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	databasev1alpha1 "opzkit/database-user-operator/api/v1alpha1"
)

// deletingDatabase returns a Database being deleted whose admin DSN
// points at a Kubernetes Secret that does not exist, so any cleanup
// attempt fails to connect.
func deletingDatabase(retain bool, status databasev1alpha1.DatabaseStatus) *databasev1alpha1.Database {
	now := metav1.Now()
	return &databasev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "app",
			Namespace:         "team",
			Finalizers:        []string{DatabaseFinalizer},
			DeletionTimestamp: &now,
		},
		Spec: databasev1alpha1.DatabaseSpec{
			Engine:         databasev1alpha1.DatabaseEnginePostgres,
			DatabaseName:   "app",
			RetainOnDelete: &retain,
			ConnectionString: databasev1alpha1.ConnectionStringSource{
				Kubernetes: &databasev1alpha1.KubernetesConnectionStringRef{Name: "missing-admin"},
			},
			SecretBackend: databasev1alpha1.SecretBackend{
				Kubernetes: &databasev1alpha1.KubernetesSecretBackend{},
			},
		},
		Status: status,
	}
}

func newDeleteReconciler(t *testing.T, db *databasev1alpha1.Database) (*DatabaseReconciler, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := databasev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(db).WithStatusSubresource(db).Build()
	return &DatabaseReconciler{Client: c, Scheme: scheme}, c
}

func TestReconcileDelete_WithoutAdminDSN(t *testing.T) {
	tests := []struct {
		name     string
		retain   bool
		status   databasev1alpha1.DatabaseStatus
		released bool
	}{
		{name: "never reached the server", status: databasev1alpha1.DatabaseStatus{}, released: true},
		// CREATE USER succeeded and the GRANT after it failed: the role
		// exists although UserCreated is false.
		{name: "partly failed create", status: databasev1alpha1.DatabaseStatus{ProvisioningStarted: true}},
		{name: "database recorded", status: databasev1alpha1.DatabaseStatus{DatabaseCreated: true}},
		{name: "user recorded", status: databasev1alpha1.DatabaseStatus{UserCreated: true}},
		{name: "secret recorded", status: databasev1alpha1.DatabaseStatus{SecretCreated: true}},
		{name: "fully provisioned", status: databasev1alpha1.DatabaseStatus{
			ProvisioningStarted: true, DatabaseCreated: true, UserCreated: true, SecretCreated: true,
		}},
		// retainOnDelete never connects, so it releases in every case.
		{name: "retain, never reached", retain: true, status: databasev1alpha1.DatabaseStatus{}, released: true},
		{name: "retain, provisioned", retain: true, status: databasev1alpha1.DatabaseStatus{
			ProvisioningStarted: true, DatabaseCreated: true, UserCreated: true, SecretCreated: true,
		}, released: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := deletingDatabase(tt.retain, tt.status)
			r, c := newDeleteReconciler(t, db)

			_, err := r.reconcileDelete(context.Background(), db)
			getErr := c.Get(context.Background(), client.ObjectKeyFromObject(db), &databasev1alpha1.Database{})

			if tt.released {
				if err != nil {
					t.Fatalf("reconcileDelete: %v", err)
				}
				// Removing the last finalizer lets the API server finish the delete.
				if !apierrors.IsNotFound(getErr) {
					t.Errorf("Database still present: %v", getErr)
				}
				return
			}
			if err == nil {
				t.Fatal("reconcileDelete succeeded without the admin DSN while resources may exist")
			}
			if getErr != nil {
				t.Errorf("Database gone, finalizer must be kept until cleanup succeeds: %v", getErr)
			}
		})
	}
}
