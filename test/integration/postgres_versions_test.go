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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	databasev1alpha1 "opzkit/database-user-operator/api/v1alpha1"
)

// PostgreSQL version compatibility matrix.
// Each version runs as its own Deployment+Service (postgres-XX) in the
// `databases` namespace, with a corresponding admin Secret
// (postgres-XX-connection) in the `default` namespace. The setup script
// applies test/integration/manifests/postgres-versions.yaml.
//
// PG 16+ tightened GRANT defaults to NOINHERIT, NOSET, NOADMIN. The
// CREATE DATABASE ... OWNER step in CreateDatabase requires the executor
// to hold SET on the owner role, so the operator's GRANT path needs
// "WITH INHERIT TRUE, SET TRUE" for vanilla PG 16/17/18. PG 14 and 15
// pre-date that change and pass either way. A regression in the GRANT
// statement therefore shows up as a failure on PG 16+ here.
var _ = Describe("PostgreSQL Version Compatibility", func() {
	const namespace = "default"

	var (
		smClient *secretsmanager.Client
		awsCtx   context.Context
	)

	BeforeEach(func() {
		awsCtx = context.Background()
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{URL: "http://localhost:14566", SigningRegion: "us-east-1"}, nil
		})
		cfg, err := config.LoadDefaultConfig(
			awsCtx,
			config.WithRegion("us-east-1"),
			config.WithEndpointResolverWithOptions(customResolver),
			config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
				return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test"}, nil
			})),
		)
		Expect(err).NotTo(HaveOccurred())
		smClient = secretsmanager.NewFromConfig(cfg)
	})

	type pgCase struct {
		version       string
		secretRefName string
	}

	cases := []pgCase{
		{version: "14", secretRefName: "postgres-14-connection"},
		{version: "15", secretRefName: "postgres-15-connection"},
		{version: "16", secretRefName: "postgres-16-connection"},
		{version: "17", secretRefName: "postgres-17-connection"},
		{version: "18", secretRefName: "postgres-18-connection"},
	}

	for _, c := range cases {
		c := c
		It(fmt.Sprintf("Should create database, user, and secret on PostgreSQL %s", c.version), func() {
			dbName := fmt.Sprintf("test-pg%s-%s", c.version, randomString(5))
			secretName := fmt.Sprintf("test/databases/%s/credentials", dbName)

			By(fmt.Sprintf("Creating Database CR targeting postgres-%s", c.version))
			createDatabase(namespace, dbName, databasev1alpha1.DatabaseSpec{
				Engine:       databasev1alpha1.DatabaseEnginePostgres,
				DatabaseName: fmt.Sprintf("pg%sdb", c.version),
				Username:     fmt.Sprintf("pg%suser", c.version),
				ConnectionString: databasev1alpha1.ConnectionStringSource{
					Kubernetes: &databasev1alpha1.KubernetesConnectionStringRef{
						Name: c.secretRefName,
						Key:  "connectionString",
					},
				},
				SecretBackend: databasev1alpha1.SecretBackend{
					AWS: &databasev1alpha1.AWSSecretBackend{
						Region: "us-east-1",
					},
				},
				SecretName: secretName,
			})

			By("Waiting for the Database to reach Ready (proves CREATE USER + GRANT + CREATE DATABASE OWNER all succeed)")
			waitForDatabaseCreated(namespace, dbName)

			By("Verifying status reflects success")
			db, err := getDatabase(namespace, dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(db.Status.DatabaseCreated).To(BeTrue())
			Expect(db.Status.UserCreated).To(BeTrue())
			Expect(db.Status.ActualUsername).To(Equal(fmt.Sprintf("pg%suser", c.version)))

			By("Verifying credentials in AWS Secrets Manager")
			Eventually(func() error {
				_, err := smClient.GetSecretValue(awsCtx, &secretsmanager.GetSecretValueInput{
					SecretId: aws.String(secretName),
				})
				return err
			}, timeout, interval).Should(Succeed())

			By("Cleaning up")
			retainFalse := false
			db.Spec.RetainOnDelete = &retainFalse
			Expect(k8sClient.Update(ctx, db)).Should(Succeed())
			Expect(deleteDatabase(namespace, dbName)).Should(Succeed())
			waitForDatabaseDeleted(namespace, dbName)
		})
	}
})
