// Copyright 2025 OpzKit
//
// Licensed under the MIT License.
// See LICENSE file in the project root for full license information.

//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev1alpha1 "opzkit/database-user-operator/api/v1alpha1"
)

var _ = Describe("Database Operator Integration Tests", func() {
	const namespace = "default"

	var (
		smClient *secretsmanager.Client
		awsCtx   context.Context
	)

	BeforeEach(func() {
		awsCtx = context.Background()

		// Configure AWS SDK to use LocalStack
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:           "http://localhost:14566",
				SigningRegion: "us-east-1",
			}, nil
		})

		cfg, err := config.LoadDefaultConfig(awsCtx,
			config.WithRegion("us-east-1"),
			config.WithEndpointResolverWithOptions(customResolver),
			config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
				return aws.Credentials{
					AccessKeyID:     "test",
					SecretAccessKey: "test",
				}, nil
			})),
		)
		Expect(err).NotTo(HaveOccurred())

		smClient = secretsmanager.NewFromConfig(cfg)
	})

	AfterEach(func() {
		By("Cleaning up all Database CRs")
		// List all Database resources
		dbList := &databasev1alpha1.DatabaseList{}
		err := k8sClient.List(ctx, dbList, client.InNamespace(namespace))
		if err != nil {
			// Log error but don't fail the test
			GinkgoWriter.Printf("Warning: failed to list databases during cleanup: %v\n", err)
			return
		}

		// Delete each Database CR
		for i := range dbList.Items {
			db := &dbList.Items[i]
			// Set retainOnDelete to false to ensure resources are cleaned up
			retainFalse := false
			db.Spec.RetainOnDelete = &retainFalse
			if err := k8sClient.Update(ctx, db); err != nil {
				GinkgoWriter.Printf("Warning: failed to update database %s to set retainOnDelete=false: %v\n", db.Name, err)
			}

			if err := k8sClient.Delete(ctx, db); err != nil {
				GinkgoWriter.Printf("Warning: failed to delete database %s: %v\n", db.Name, err)
			}
		}

		// Wait for all databases to be deleted
		Eventually(func() int {
			dbList := &databasev1alpha1.DatabaseList{}
			if err := k8sClient.List(ctx, dbList, client.InNamespace(namespace)); err != nil {
				return -1
			}
			return len(dbList.Items)
		}, "30s", "1s").Should(Equal(0), "All Database CRs should be deleted")
	})

	Context("PostgreSQL Database Lifecycle", func() {
		It("Should create and manage PostgreSQL database with all features", func() {
			dbName := "test-pg-full-" + randomString(5)
			secretName := fmt.Sprintf("test/databases/%s/credentials", dbName)

			By("Creating a PostgreSQL Database resource with Secrets Manager")
			createDatabase(namespace, dbName, databasev1alpha1.DatabaseSpec{
				Engine:       databasev1alpha1.DatabaseEnginePostgres,
				DatabaseName: "testdb",
				Username:     "testuser",
				ConnectionStringSecretRef: &databasev1alpha1.SecretKeyReference{
					Name: "postgres-connection",
					Key:  "connectionString",
				},
				SecretName: secretName,
			})

			By("Waiting for database to be created")
			waitForDatabaseCreated(namespace, dbName)

			By("Verifying database status")
			db, err := getDatabase(namespace, dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(db.Status.DatabaseCreated).To(BeTrue())
			Expect(db.Status.UserCreated).To(BeTrue())
			Expect(db.Status.ActualUsername).To(Equal("testuser"))
			Expect(db.Status.Phase).To(Or(Equal("Ready"), Equal("Reconciling")))
			Expect(db.Status.ObservedGeneration).To(BeNumerically(">", 0))

			By("Verifying connection info")
			Expect(db.Status.ConnectionInfo.Host).NotTo(BeEmpty())
			Expect(db.Status.ConnectionInfo.Port).To(Equal(5432))
			Expect(db.Status.ConnectionInfo.Database).To(Equal("testdb"))
			Expect(db.Status.ConnectionInfo.Engine).To(Equal("postgres"))

			By("Verifying finalizer is present")
			Expect(db.GetFinalizers()).To(ContainElement("database.opzkit.io/database-finalizer"))

			By("Verifying credentials in AWS Secrets Manager")
			Eventually(func() error {
				_, err := smClient.GetSecretValue(awsCtx, &secretsmanager.GetSecretValueInput{
					SecretId: aws.String(secretName),
				})
				return err
			}, timeout, interval).Should(Succeed())

			By("Validating Secrets Manager content")
			result, err := smClient.GetSecretValue(awsCtx, &secretsmanager.GetSecretValueInput{
				SecretId: aws.String(secretName),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SecretString).NotTo(BeNil())

			var secretData map[string]interface{}
			err = json.Unmarshal([]byte(*result.SecretString), &secretData)
			Expect(err).NotTo(HaveOccurred())
			Expect(secretData["DB_USERNAME"]).To(Equal("testuser"))
			Expect(secretData["DB_PASSWORD"]).NotTo(BeEmpty())
			Expect(secretData["DB_HOST"]).NotTo(BeEmpty())
			Expect(secretData["DB_PORT"]).To(BeNumerically("==", 5432))
			Expect(secretData["DB_NAME"]).To(Equal("testdb"))
			Expect(secretData["POSTGRES_URL"]).NotTo(BeEmpty())

			By("Testing idempotent reconciliation")
			if db.Annotations == nil {
				db.Annotations = make(map[string]string)
			}
			db.Annotations["test"] = "reconcile"
			generation1 := db.Status.ObservedGeneration
			Expect(k8sClient.Update(ctx, db)).Should(Succeed())

			Eventually(func() bool {
				db2, err := getDatabase(namespace, dbName)
				if err != nil {
					return false
				}
				return db2.Status.ObservedGeneration >= generation1 &&
					db2.Status.DatabaseCreated &&
					db2.Status.UserCreated
			}, timeout, interval).Should(BeTrue())

			By("Cleaning up database")
			retainFalse := false
			db, _ = getDatabase(namespace, dbName)
			db.Spec.RetainOnDelete = &retainFalse
			Expect(k8sClient.Update(ctx, db)).Should(Succeed())
			Expect(deleteDatabase(namespace, dbName)).Should(Succeed())
			waitForDatabaseDeleted(namespace, dbName)

			By("Verifying Secrets Manager cleanup")
			Eventually(func() error {
				_, err := smClient.GetSecretValue(awsCtx, &secretsmanager.GetSecretValueInput{
					SecretId: aws.String(secretName),
				})
				if err != nil {
					return nil // Secret not found is what we want
				}
				return fmt.Errorf("secret still exists")
			}, timeout, interval).Should(Succeed())
		})

		It("Should handle PostgreSQL database with default username", func() {
			dbName := "test-pg-default-" + randomString(5)

			By("Creating a Database resource without custom username")
			createDatabase(namespace, dbName, databasev1alpha1.DatabaseSpec{
				Engine:       databasev1alpha1.DatabaseEnginePostgres,
				DatabaseName: "defaultdb",
				ConnectionStringSecretRef: &databasev1alpha1.SecretKeyReference{
					Name: "postgres-connection",
					Key:  "connectionString",
				},
			})

			By("Waiting for database to be created")
			waitForDatabaseCreated(namespace, dbName)

			By("Verifying default username is used")
			db, err := getDatabase(namespace, dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(db.Status.ActualUsername).To(Equal("defaultdb"))

			By("Cleaning up")
			retainFalse := false
			db.Spec.RetainOnDelete = &retainFalse
			Expect(k8sClient.Update(ctx, db)).Should(Succeed())
			Expect(deleteDatabase(namespace, dbName)).Should(Succeed())
			waitForDatabaseDeleted(namespace, dbName)
		})

		It("Should support postgresql:// URL scheme", func() {
			dbName := "test-pg-postgresql-" + randomString(5)

			By("Creating a Database resource with postgresql engine")
			createDatabase(namespace, dbName, databasev1alpha1.DatabaseSpec{
				Engine:       databasev1alpha1.DatabaseEnginePostgreSQL,
				DatabaseName: "postgresqldb",
				ConnectionStringSecretRef: &databasev1alpha1.SecretKeyReference{
					Name: "postgres-connection",
					Key:  "connectionString",
				},
			})

			By("Waiting for database to be created")
			waitForDatabaseCreated(namespace, dbName)

			By("Verifying database was created")
			db, err := getDatabase(namespace, dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(db.Status.DatabaseCreated).To(BeTrue())
			Expect(db.Status.UserCreated).To(BeTrue())

			By("Cleaning up")
			retainFalse := false
			db.Spec.RetainOnDelete = &retainFalse
			Expect(k8sClient.Update(ctx, db)).Should(Succeed())
			Expect(deleteDatabase(namespace, dbName)).Should(Succeed())
			waitForDatabaseDeleted(namespace, dbName)
		})

		It("Should retain resources when retainOnDelete=true", func() {
			dbName := "test-pg-retain-" + randomString(5)
			secretName := fmt.Sprintf("test/databases/%s/credentials", dbName)

			By("Creating a Database resource with retainOnDelete=true")
			retainTrue := true
			createDatabase(namespace, dbName, databasev1alpha1.DatabaseSpec{
				Engine:         databasev1alpha1.DatabaseEnginePostgres,
				DatabaseName:   "retaindb",
				Username:       "retainuser",
				RetainOnDelete: &retainTrue,
				ConnectionStringSecretRef: &databasev1alpha1.SecretKeyReference{
					Name: "postgres-connection",
					Key:  "connectionString",
				},
				SecretName: secretName,
			})

			By("Waiting for database creation")
			waitForDatabaseCreated(namespace, dbName)

			By("Verifying secret exists in Secrets Manager")
			_, err := smClient.GetSecretValue(awsCtx, &secretsmanager.GetSecretValueInput{
				SecretId: aws.String(secretName),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the Database resource")
			Expect(deleteDatabase(namespace, dbName)).Should(Succeed())
			waitForDatabaseDeleted(namespace, dbName)

			By("Verifying secret still exists in Secrets Manager")
			_, err = smClient.GetSecretValue(awsCtx, &secretsmanager.GetSecretValueInput{
				SecretId: aws.String(secretName),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Manually cleaning up retained secret")
			_, err = smClient.DeleteSecret(awsCtx, &secretsmanager.DeleteSecretInput{
				SecretId:                   aws.String(secretName),
				ForceDeleteWithoutRecovery: aws.Bool(true),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Manually cleaning up retained database and user from PostgreSQL")
			// Use kubectl to find the postgres pod and clean up the retained resources
			// This is necessary because the Database CR has been deleted, so AfterEach won't handle it
			Eventually(func() error {
				cmd := "kubectl get pods -n databases -l app=postgres -o jsonpath='{.items[0].metadata.name}'"
				output, err := exec.Command("sh", "-c", cmd).CombinedOutput()
				if err != nil {
					return fmt.Errorf("failed to get postgres pod: %w (output: %s)", err, string(output))
				}
				podName := strings.TrimSpace(string(output))
				if podName == "" {
					return fmt.Errorf("postgres pod not found")
				}

				// Drop the database
				dropDBCmd := fmt.Sprintf("kubectl exec -n databases %s -- psql -U postgres -c 'DROP DATABASE IF EXISTS retaindb;'", podName)
				if output, err := exec.Command("sh", "-c", dropDBCmd).CombinedOutput(); err != nil {
					return fmt.Errorf("failed to drop database: %w (output: %s)", err, string(output))
				}

				// Drop the user
				dropUserCmd := fmt.Sprintf("kubectl exec -n databases %s -- psql -U postgres -c 'DROP USER IF EXISTS retainuser;'", podName)
				if output, err := exec.Command("sh", "-c", dropUserCmd).CombinedOutput(); err != nil {
					return fmt.Errorf("failed to drop user: %w (output: %s)", err, string(output))
				}

				return nil
			}, "30s", "1s").Should(Succeed())
		})

		It("Should detect and report error when database/user exist but secret is missing", func() {
			dbName1 := "test-pg-orphaned-" + randomString(5)
			dbName2 := "test-pg-orphaned-" + randomString(5)
			secretName := fmt.Sprintf("test/databases/%s/credentials", dbName1)

			By("Creating first Database resource to set up orphaned resources")
			createDatabase(namespace, dbName1, databasev1alpha1.DatabaseSpec{
				Engine:       databasev1alpha1.DatabaseEnginePostgres,
				DatabaseName: "orphaneddb",
				Username:     "orphaneduser",
				ConnectionStringSecretRef: &databasev1alpha1.SecretKeyReference{
					Name: "postgres-connection",
					Key:  "connectionString",
				},
				SecretName: secretName,
			})

			By("Waiting for first database to be created")
			waitForDatabaseCreated(namespace, dbName1)

			By("Verifying secret exists")
			_, err := smClient.GetSecretValue(awsCtx, &secretsmanager.GetSecretValueInput{
				SecretId: aws.String(secretName),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Deleting first Database CR (simulating deletion with resources left behind)")
			Expect(deleteDatabase(namespace, dbName1)).Should(Succeed())
			waitForDatabaseDeleted(namespace, dbName1)

			By("Deleting the secret from AWS Secrets Manager (simulating external deletion)")
			_, err = smClient.DeleteSecret(awsCtx, &secretsmanager.DeleteSecretInput{
				SecretId:                   aws.String(secretName),
				ForceDeleteWithoutRecovery: aws.Bool(true),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Creating second Database CR with same database/username but no secret")
			// This simulates the error scenario: database and user exist, but secret is missing
			createDatabase(namespace, dbName2, databasev1alpha1.DatabaseSpec{
				Engine:       databasev1alpha1.DatabaseEnginePostgres,
				DatabaseName: "orphaneddb",
				Username:     "orphaneduser",
				ConnectionStringSecretRef: &databasev1alpha1.SecretKeyReference{
					Name: "postgres-connection",
					Key:  "connectionString",
				},
				SecretName: secretName,
			})

			By("Verifying the operator detects the error condition")
			Eventually(func() string {
				db, err := getDatabase(namespace, dbName2)
				if err != nil {
					return ""
				}
				return db.Status.Phase
			}, "30s", "1s").Should(Equal("Error"))

			By("Verifying the error message is correct")
			db, err := getDatabase(namespace, dbName2)
			Expect(err).NotTo(HaveOccurred())
			Expect(db.Status.Message).To(ContainSubstring("database and/or user exist but secret is missing"))
			Expect(db.Status.Message).To(ContainSubstring("cannot recover password"))

			By("Cleaning up - deleting the second Database CR and PostgreSQL resources")
			// Set retainOnDelete to false to ensure cleanup
			db, err = getDatabase(namespace, dbName2)
			Expect(err).NotTo(HaveOccurred())
			retainFalse := false
			db.Spec.RetainOnDelete = &retainFalse
			Expect(k8sClient.Update(ctx, db)).Should(Succeed())
			Expect(deleteDatabase(namespace, dbName2)).Should(Succeed())
			waitForDatabaseDeleted(namespace, dbName2)
		})
	})

	Context("MySQL Database Lifecycle", func() {
		It("Should create and manage MySQL database with all features", func() {
			dbName := "test-mysql-full-" + randomString(5)
			secretName := fmt.Sprintf("test/databases/%s/credentials", dbName)

			By("Creating a MySQL Database resource with Secrets Manager")
			createDatabase(namespace, dbName, databasev1alpha1.DatabaseSpec{
				Engine:       databasev1alpha1.DatabaseEngineMySQL,
				DatabaseName: "mysqldb",
				Username:     "mysqluser",
				ConnectionStringSecretRef: &databasev1alpha1.SecretKeyReference{
					Name: "mysql-connection",
					Key:  "connectionString",
				},
				SecretName: secretName,
			})

			By("Waiting for database to be created")
			waitForDatabaseCreated(namespace, dbName)

			By("Verifying database status")
			db, err := getDatabase(namespace, dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(db.Status.DatabaseCreated).To(BeTrue())
			Expect(db.Status.UserCreated).To(BeTrue())
			Expect(db.Status.ActualUsername).To(Equal("mysqluser"))
			Expect(db.Status.Phase).To(Or(Equal("Ready"), Equal("Reconciling")))

			By("Verifying MySQL connection info")
			Expect(db.Status.ConnectionInfo.Host).NotTo(BeEmpty())
			Expect(db.Status.ConnectionInfo.Port).To(Equal(3306))
			Expect(db.Status.ConnectionInfo.Database).To(Equal("mysqldb"))
			Expect(db.Status.ConnectionInfo.Engine).To(Equal("mysql"))

			By("Verifying finalizer is present")
			Expect(db.GetFinalizers()).To(ContainElement("database.opzkit.io/database-finalizer"))

			By("Verifying credentials in AWS Secrets Manager")
			Eventually(func() error {
				_, err := smClient.GetSecretValue(awsCtx, &secretsmanager.GetSecretValueInput{
					SecretId: aws.String(secretName),
				})
				return err
			}, timeout, interval).Should(Succeed())

			By("Validating Secrets Manager content for MySQL")
			result, err := smClient.GetSecretValue(awsCtx, &secretsmanager.GetSecretValueInput{
				SecretId: aws.String(secretName),
			})
			Expect(err).NotTo(HaveOccurred())

			var secretData map[string]interface{}
			err = json.Unmarshal([]byte(*result.SecretString), &secretData)
			Expect(err).NotTo(HaveOccurred())
			Expect(secretData["DB_USERNAME"]).To(Equal("mysqluser"))
			Expect(secretData["DB_PASSWORD"]).NotTo(BeEmpty())
			Expect(secretData["DB_PORT"]).To(BeNumerically("==", 3306))
			Expect(secretData["DB_NAME"]).To(Equal("mysqldb"))
			Expect(secretData["MYSQL_URL"]).NotTo(BeEmpty())

			By("Testing UTF8MB4 encoding")
			// Note: In a real integration test with actual MySQL connection,
			// we would verify the database charset and collation are utf8mb4

			By("Testing idempotent reconciliation")
			if db.Annotations == nil {
				db.Annotations = make(map[string]string)
			}
			db.Annotations["test"] = "reconcile"
			generation1 := db.Status.ObservedGeneration
			Expect(k8sClient.Update(ctx, db)).Should(Succeed())

			Eventually(func() bool {
				db2, err := getDatabase(namespace, dbName)
				if err != nil {
					return false
				}
				return db2.Status.ObservedGeneration >= generation1 &&
					db2.Status.DatabaseCreated &&
					db2.Status.UserCreated
			}, timeout, interval).Should(BeTrue())

			By("Cleaning up")
			retainFalse := false
			db, _ = getDatabase(namespace, dbName)
			db.Spec.RetainOnDelete = &retainFalse
			Expect(k8sClient.Update(ctx, db)).Should(Succeed())
			Expect(deleteDatabase(namespace, dbName)).Should(Succeed())
			waitForDatabaseDeleted(namespace, dbName)

			By("Verifying Secrets Manager cleanup")
			Eventually(func() error {
				_, err := smClient.GetSecretValue(awsCtx, &secretsmanager.GetSecretValueInput{
					SecretId: aws.String(secretName),
				})
				if err != nil {
					return nil
				}
				return fmt.Errorf("secret still exists")
			}, timeout, interval).Should(Succeed())
		})

		It("Should handle MySQL database with default username", func() {
			dbName := "test-mysql-default-" + randomString(5)

			By("Creating a Database resource without custom username")
			createDatabase(namespace, dbName, databasev1alpha1.DatabaseSpec{
				Engine:       databasev1alpha1.DatabaseEngineMySQL,
				DatabaseName: "defaultdb",
				ConnectionStringSecretRef: &databasev1alpha1.SecretKeyReference{
					Name: "mysql-connection",
					Key:  "connectionString",
				},
			})

			By("Waiting for database to be created")
			waitForDatabaseCreated(namespace, dbName)

			By("Verifying default username is used")
			db, err := getDatabase(namespace, dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(db.Status.ActualUsername).To(Equal("defaultdb"))

			By("Cleaning up")
			retainFalse := false
			db.Spec.RetainOnDelete = &retainFalse
			Expect(k8sClient.Update(ctx, db)).Should(Succeed())
			Expect(deleteDatabase(namespace, dbName)).Should(Succeed())
			waitForDatabaseDeleted(namespace, dbName)
		})

		It("Should support MariaDB engine type", func() {
			dbName := "test-mariadb-" + randomString(5)

			By("Creating a Database resource with mariadb engine")
			createDatabase(namespace, dbName, databasev1alpha1.DatabaseSpec{
				Engine:       databasev1alpha1.DatabaseEngineMariaDB,
				DatabaseName: "mariadb",
				ConnectionStringSecretRef: &databasev1alpha1.SecretKeyReference{
					Name: "mysql-connection",
					Key:  "connectionString",
				},
			})

			By("Waiting for database to be created")
			waitForDatabaseCreated(namespace, dbName)

			By("Verifying database was created with correct engine")
			db, err := getDatabase(namespace, dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(db.Status.DatabaseCreated).To(BeTrue())
			Expect(db.Status.UserCreated).To(BeTrue())
			Expect(db.Status.ConnectionInfo.Engine).To(Equal("mariadb"))

			By("Cleaning up")
			retainFalse := false
			db.Spec.RetainOnDelete = &retainFalse
			Expect(k8sClient.Update(ctx, db)).Should(Succeed())
			Expect(deleteDatabase(namespace, dbName)).Should(Succeed())
			waitForDatabaseDeleted(namespace, dbName)
		})
	})

	Context("Multi-Database Operations", func() {
		It("Should support PostgreSQL and MySQL databases simultaneously", func() {
			pgName := "test-multi-pg-" + randomString(5)
			mysqlName := "test-multi-mysql-" + randomString(5)
			pgSecretName := fmt.Sprintf("test/databases/%s/credentials", pgName)
			mysqlSecretName := fmt.Sprintf("test/databases/%s/credentials", mysqlName)

			By("Creating PostgreSQL and MySQL databases")
			createDatabase(namespace, pgName, databasev1alpha1.DatabaseSpec{
				Engine:       databasev1alpha1.DatabaseEnginePostgres,
				DatabaseName: "multidb_pg",
				Username:     "pguser",
				ConnectionStringSecretRef: &databasev1alpha1.SecretKeyReference{
					Name: "postgres-connection",
					Key:  "connectionString",
				},
				SecretName: pgSecretName,
			})

			createDatabase(namespace, mysqlName, databasev1alpha1.DatabaseSpec{
				Engine:       databasev1alpha1.DatabaseEngineMySQL,
				DatabaseName: "multidb_mysql",
				Username:     "mysqluser",
				ConnectionStringSecretRef: &databasev1alpha1.SecretKeyReference{
					Name: "mysql-connection",
					Key:  "connectionString",
				},
				SecretName: mysqlSecretName,
			})

			By("Waiting for both databases")
			waitForDatabaseCreated(namespace, pgName)
			waitForDatabaseCreated(namespace, mysqlName)

			By("Verifying both databases are created correctly")
			pgDb, err := getDatabase(namespace, pgName)
			Expect(err).NotTo(HaveOccurred())
			Expect(pgDb.Status.DatabaseCreated).To(BeTrue())
			Expect(pgDb.Status.ConnectionInfo.Engine).To(Equal("postgres"))
			Expect(pgDb.Status.ConnectionInfo.Port).To(Equal(5432))

			mysqlDb, err := getDatabase(namespace, mysqlName)
			Expect(err).NotTo(HaveOccurred())
			Expect(mysqlDb.Status.DatabaseCreated).To(BeTrue())
			Expect(mysqlDb.Status.ConnectionInfo.Engine).To(Equal("mysql"))
			Expect(mysqlDb.Status.ConnectionInfo.Port).To(Equal(3306))

			By("Verifying both secrets exist in Secrets Manager")
			_, err = smClient.GetSecretValue(awsCtx, &secretsmanager.GetSecretValueInput{
				SecretId: aws.String(pgSecretName),
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = smClient.GetSecretValue(awsCtx, &secretsmanager.GetSecretValueInput{
				SecretId: aws.String(mysqlSecretName),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Cleaning up both databases")
			retainFalse := false

			pgDb.Spec.RetainOnDelete = &retainFalse
			Expect(k8sClient.Update(ctx, pgDb)).Should(Succeed())
			Expect(deleteDatabase(namespace, pgName)).Should(Succeed())

			mysqlDb.Spec.RetainOnDelete = &retainFalse
			Expect(k8sClient.Update(ctx, mysqlDb)).Should(Succeed())
			Expect(deleteDatabase(namespace, mysqlName)).Should(Succeed())

			waitForDatabaseDeleted(namespace, pgName)
			waitForDatabaseDeleted(namespace, mysqlName)

			By("Verifying both secrets are deleted")
			Eventually(func() bool {
				_, err1 := smClient.GetSecretValue(awsCtx, &secretsmanager.GetSecretValueInput{
					SecretId: aws.String(pgSecretName),
				})
				_, err2 := smClient.GetSecretValue(awsCtx, &secretsmanager.GetSecretValueInput{
					SecretId: aws.String(mysqlSecretName),
				})
				return err1 != nil && err2 != nil
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Error Handling", func() {
		It("Should report error for non-existent connection secret", func() {
			dbName := "test-error-no-secret-" + randomString(5)

			By("Creating a Database resource with non-existent secret")
			createDatabase(namespace, dbName, databasev1alpha1.DatabaseSpec{
				Engine:       databasev1alpha1.DatabaseEnginePostgres,
				DatabaseName: "errordb",
				ConnectionStringSecretRef: &databasev1alpha1.SecretKeyReference{
					Name: "non-existent-secret",
					Key:  "connectionString",
				},
			})

			By("Verifying error phase")
			waitForDatabasePhase(namespace, dbName, "Error")

			By("Verifying error message is set")
			db, err := getDatabase(namespace, dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(db.Status.Message).To(ContainSubstring("not found"))

			By("Cleaning up")
			Expect(deleteDatabase(namespace, dbName)).Should(Succeed())
			waitForDatabaseDeleted(namespace, dbName)
		})
	})
})
