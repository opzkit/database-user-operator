/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	databasev1alpha1 "opzkit/database-user-operator/api/v1alpha1"
	"opzkit/database-user-operator/internal/database"
	"opzkit/database-user-operator/internal/secrets"
)

const (
	DatabaseFinalizer = "database.opzkit.io/database-finalizer"

	// Requeue interval for successful reconciliation
	requeueAfterSuccess = 10 * time.Minute
)

// DatabaseReconciler reconciles a Database object
type DatabaseReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=database.opzkit.io,resources=databases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.opzkit.io,resources=databases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=database.opzkit.io,resources=databases/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting reconciliation")

	db := &databasev1alpha1.Database{}
	if err := r.Get(ctx, req.NamespacedName, db); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Record creation event on first reconciliation
	if db.Status.ObservedGeneration == 0 {
		r.Recorder.Event(db, corev1.EventTypeNormal, "Created", "Database created")
	}

	// Handle deletion
	if !db.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, db)
	}

	// Add finalizer if needed
	if !controllerutil.ContainsFinalizer(db, DatabaseFinalizer) {
		controllerutil.AddFinalizer(db, DatabaseFinalizer)
		if err := r.Update(ctx, db); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Perform reconciliation
	err := r.reconcileDatabase(ctx, db)

	// Update status based on result
	statusChanged := false
	if err != nil {
		// Normalize error message to avoid status updates due to dynamic content (RequestIDs, etc.)
		normalizedErrMsg := normalizeErrorMessage(err.Error())
		if db.Status.Phase != "Error" || db.Status.Message != normalizedErrMsg {
			db.Status.Phase = "Error"
			db.Status.Message = normalizedErrMsg
			db.Status.ObservedGeneration = db.Generation
			statusChanged = true
		}

		// Update status only if it changed to avoid triggering unnecessary reconciliations
		if statusChanged {
			if statusErr := r.Status().Update(ctx, db); statusErr != nil {
				logger.Error(statusErr, "Failed to update error status")
			}
		}

		// Record event for user visibility (only once per error by checking if status changed)
		if statusChanged {
			if apierrors.IsNotFound(err) {
				r.Recorder.Event(db, corev1.EventTypeWarning, "ConfigurationError", err.Error())
			} else if isAWSPermissionError(err) {
				r.Recorder.Event(db, corev1.EventTypeWarning, "PermissionError",
					"AWS permission denied. Ensure the operator has IAM permissions for Secrets Manager. "+
						"Grant secretsmanager:* on the secret ARN, or configure IRSA/instance profile.")
			} else if isAWSResourceNotFoundError(err) {
				r.Recorder.Event(db, corev1.EventTypeWarning, "ResourceNotFound",
					"AWS resource not found. Verify the secret exists in AWS Secrets Manager and the name/region are correct in the Database spec.")
			} else {
				r.Recorder.Event(db, corev1.EventTypeWarning, "ReconciliationError", err.Error())
			}
		}

		// Handle AWS permission errors with longer backoff since they require manual intervention
		if isAWSPermissionError(err) {
			logger.Error(err, "AWS permission error - requires IAM policy update",
				"action", "Update IAM policy or IRSA configuration to grant Secrets Manager permissions",
				"requeueAfter", "5m")
			// Use 5 minute backoff to prevent log spam while allowing automatic recovery
			return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
		}

		// Handle ResourceNotFoundException with longer backoff since it requires manual intervention
		if isAWSResourceNotFoundError(err) {
			logger.Error(err, "AWS resource not found - requires manual configuration",
				"action", "Verify secret exists in AWS Secrets Manager and the name/region are correct",
				"requeueAfter", "5m")
			// Use 5 minute backoff to prevent log spam while allowing automatic recovery
			return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
		}

		// Return error to trigger rate limiter's exponential backoff
		// Note: controller-runtime automatically logs reconciler errors, no need to log here
		return ctrl.Result{}, err
	}

	// Success - always update status to persist resource creation flags and ObservedGeneration
	db.Status.Phase = "Ready"
	db.Status.Message = "Database, user, and secret are ready"
	db.Status.ObservedGeneration = db.Generation
	if err := r.Status().Update(ctx, db); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciliation successful",
		"database", db.Spec.DatabaseName,
		"username", db.Status.ActualUsername,
		"secretName", db.Status.ActualSecretName,
		"secretARN", db.Status.SecretARN,
		"requeueAfter", requeueAfterSuccess)
	return ctrl.Result{RequeueAfter: requeueAfterSuccess}, nil
}

func (r *DatabaseReconciler) reconcileDatabase(ctx context.Context, db *databasev1alpha1.Database) error {
	logger := log.FromContext(ctx)

	// Check if reconciliation is needed
	if !needsReconciliation(db) {
		logger.Info("Resources already exist and spec unchanged, skipping reconciliation",
			"database", db.Spec.DatabaseName,
			"username", db.Status.ActualUsername,
			"secretName", db.Status.ActualSecretName,
			"generation", db.Generation)
		return nil
	}

	const currentSecretFormatVersion = "v2"
	needsSecretUpdate := db.Status.SecretFormatVersion != currentSecretFormatVersion

	if needsSecretUpdate {
		logger.Info("Secret format needs updating",
			"database", db.Spec.DatabaseName,
			"currentFormat", db.Status.SecretFormatVersion,
			"targetFormat", currentSecretFormatVersion)
	}

	logger.Info("Reconciling database resources",
		"database", db.Spec.DatabaseName,
		"userCreated", db.Status.UserCreated,
		"databaseCreated", db.Status.DatabaseCreated,
		"secretCreated", db.Status.SecretCreated,
		"secretFormatVersion", db.Status.SecretFormatVersion,
		"generation", db.Generation,
		"observedGeneration", db.Status.ObservedGeneration)

	connectionString, err := r.getConnectionString(ctx, db)
	if err != nil {
		return err
	}

	dbClient, err := database.NewClient(string(db.Spec.Engine), connectionString)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := dbClient.Close(); closeErr != nil {
			logger := log.FromContext(ctx)
			logger.Error(closeErr, "Failed to close database connection")
		}
	}()

	// Get connection info from the client
	connInfo := dbClient.GetConnectionInfo()

	username := getUsernameOrDefault(db)

	var password string

	// If only updating secret format (user/db already exist), retrieve existing password from AWS
	if needsSecretUpdate && db.Status.UserCreated && db.Status.DatabaseCreated && db.Status.SecretCreated {
		region := r.getRegion(db)

		// Validate region
		if err := secrets.ValidateRegion(region); err != nil {
			return fmt.Errorf("invalid AWS region for password retrieval: %w", err)
		}

		logger.Info("Retrieving existing password from AWS Secrets Manager for format migration",
			"secretName", db.Status.ActualSecretName,
			"region", region)

		awsClient, err := secrets.NewAWSSecretsManagerClient(ctx, region)
		if err != nil {
			return fmt.Errorf("failed to create AWS client for password retrieval: %w", err)
		}

		existingSecret, err := awsClient.GetSecret(ctx, db.Status.ActualSecretName)
		if err != nil {
			return fmt.Errorf("failed to retrieve existing secret for migration: %w", err)
		}

		password = existingSecret.DBPassword
		if password == "" {
			return fmt.Errorf("could not extract password from existing secret for migration")
		}

		logger.Info("Successfully retrieved existing password for secret format migration")
	} else {
		// Check actual existence of resources in database
		userExists, err := dbClient.UserExists(ctx, username)
		if err != nil {
			return fmt.Errorf("failed to check if user exists: %w", err)
		}

		dbExists, err := dbClient.DatabaseExists(ctx, db.Spec.DatabaseName)
		if err != nil {
			return fmt.Errorf("failed to check if database exists: %w", err)
		}

		// Check if secret exists in AWS Secrets Manager
		var secretExists bool
		var awsClient *secrets.AWSSecretsManagerClient
		region := r.getRegion(db)

		// Validate region
		if err := secrets.ValidateRegion(region); err != nil {
			return fmt.Errorf("invalid AWS region: %w", err)
		}

		awsClient, err = secrets.NewAWSSecretsManagerClient(ctx, region)
		if err != nil {
			return fmt.Errorf("failed to create AWS client: %w", err)
		}

		// Get the actual resolved region from the AWS client
		region = awsClient.GetRegion()

		// Determine secret name
		secretName := getSecretNameOrDefault(db)

		secretExists, err = awsClient.SecretExists(ctx, secretName)
		if err != nil {
			return fmt.Errorf("failed to check if secret exists: %w", err)
		}

		logger.Info("Checked resource existence",
			"userExists", userExists,
			"databaseExists", dbExists,
			"secretExists", secretExists,
			"secretName", secretName)

		// Decision logic based on resource existence
		if dbExists && userExists && secretExists {
			// All three exist - nothing to do, just verify and update status
			logger.Info("Database, user, and secret already exist - skipping creation",
				"database", db.Spec.DatabaseName,
				"username", username,
				"secretName", secretName)

			// Retrieve password from secret for grant operations
			existingSecret, err := awsClient.GetSecret(ctx, secretName)
			if err != nil {
				return fmt.Errorf("failed to retrieve existing secret: %w", err)
			}
			password = existingSecret.DBPassword
			if password == "" {
				return fmt.Errorf("could not extract password from existing secret")
			}

			// Always update tags to ensure they're in sync with spec
			desiredTags := map[string]string{"ManagedBy": "database-user-operator"}
			if db.Spec.AWSSecretsManager != nil {
				for k, v := range db.Spec.AWSSecretsManager.Tags {
					desiredTags[k] = v
				}
			}

			// Get existing tags to determine what needs to be removed
			existingTags, err := awsClient.GetSecretTags(ctx, secretName)
			if err != nil {
				logger.Error(err, "Failed to get existing secret tags, will still attempt to update tags",
					"secretName", secretName)
				existingTags = map[string]string{} // Continue with empty set
			}

			// Determine tags to remove (exist but not in desired)
			var tagsToRemove []string
			for existingKey := range existingTags {
				if _, desired := desiredTags[existingKey]; !desired {
					tagsToRemove = append(tagsToRemove, existingKey)
				}
			}

			// Remove unwanted tags
			if len(tagsToRemove) > 0 {
				logger.Info("Removing tags from secret in AWS Secrets Manager",
					"secretName", secretName,
					"tagsToRemove", tagsToRemove)
				if err := awsClient.UntagSecret(ctx, secretName, tagsToRemove); err != nil {
					return fmt.Errorf("failed to remove secret tags: %w", err)
				}
			}

			// Add or update desired tags
			logger.Info("Updating secret tags in AWS Secrets Manager",
				"secretName", secretName,
				"desiredTags", desiredTags)
			if err := awsClient.TagSecret(ctx, secretName, desiredTags); err != nil {
				return fmt.Errorf("failed to update secret tags: %w", err)
			}

			// Update status
			db.Status.UserCreated = true
			db.Status.DatabaseCreated = true
			db.Status.SecretCreated = true
			db.Status.ActualUsername = username
			db.Status.ActualSecretName = secretName

		} else if (dbExists || userExists) && !secretExists {
			// Database and/or user exist but secret is missing
			// Check if this is a region change scenario
			regionChanged := db.Status.SecretRegion != "" && db.Status.SecretRegion != region

			if regionChanged && db.Status.ActualSecretName != "" {
				logger.Info("Region change detected - attempting to retrieve password from old region",
					"oldRegion", db.Status.SecretRegion,
					"newRegion", region,
					"secretName", db.Status.ActualSecretName)

				// Try to get password from old region
				oldRegionClient, err := secrets.NewAWSSecretsManagerClient(ctx, db.Status.SecretRegion)
				if err != nil {
					return fmt.Errorf("failed to create AWS client for old region (%s): %w", db.Status.SecretRegion, err)
				}

				oldSecretExists, err := oldRegionClient.SecretExists(ctx, db.Status.ActualSecretName)
				if err != nil {
					return fmt.Errorf("failed to check if secret exists in old region: %w", err)
				}

				if oldSecretExists {
					logger.Info("Found secret in old region, retrieving password",
						"oldRegion", db.Status.SecretRegion,
						"secretName", db.Status.ActualSecretName)

					oldSecret, err := oldRegionClient.GetSecret(ctx, db.Status.ActualSecretName)
					if err != nil {
						return fmt.Errorf("failed to retrieve secret from old region (%s): %w", db.Status.SecretRegion, err)
					}

					password = oldSecret.DBPassword
					if password == "" {
						return fmt.Errorf("could not extract password from secret in old region")
					}

					logger.Info("Successfully retrieved password from old region, will create secret in new region",
						"oldRegion", db.Status.SecretRegion,
						"newRegion", region)

					// Update status to reflect we have the resources
					db.Status.UserCreated = true
					db.Status.DatabaseCreated = true
					db.Status.ActualUsername = username
					db.Status.ActualSecretName = secretName
					// Note: SecretCreated will remain false until we actually create it in the new region
				} else {
					return fmt.Errorf("region changed from %s to %s but secret not found in old region - cannot recover password. Please delete the Database CR and recreate it, or manually create the secret with the correct password",
						db.Status.SecretRegion, region)
				}
			} else {
				// Not a region change - this is an unrecoverable error
				return fmt.Errorf("database and/or user exist but secret is missing - cannot recover password (database exists: %v, user exists: %v, secret exists: %v). Please delete the Database CR and recreate it, or manually create the secret with the correct password",
					dbExists, userExists, secretExists)
			}

		} else {
			// Create missing resources

			// Generate new password for new resources
			password, err = database.GeneratePassword(32)
			if err != nil {
				return err
			}

			// Create user if doesn't exist
			if !userExists {
				logger.Info("Creating new database user",
					"database", db.Spec.DatabaseName,
					"username", username,
					"host", connInfo.Host)

				if err := dbClient.CreateUser(ctx, username, password); err != nil {
					return err
				}
				logger.Info("Database user created successfully",
					"username", username)
			} else {
				logger.Info("User already exists",
					"username", username)
			}
			db.Status.UserCreated = true
			db.Status.ActualUsername = username

			// Create database if doesn't exist
			if !dbExists {
				logger.Info("Creating new database",
					"database", db.Spec.DatabaseName,
					"owner", username,
					"host", connInfo.Host)

				if err := dbClient.CreateDatabase(ctx, db.Spec.DatabaseName, username); err != nil {
					return err
				}
				logger.Info("Database created successfully",
					"database", db.Spec.DatabaseName)
			} else {
				logger.Info("Database already exists",
					"database", db.Spec.DatabaseName)
			}
			db.Status.DatabaseCreated = true
		}
	}

	// Ensure we have the actual username set (needed for secret format migration)
	if db.Status.ActualUsername == "" {
		db.Status.ActualUsername = username
	}

	privileges := db.Spec.Privileges
	if len(privileges) == 0 {
		privileges = []string{"ALL"}
	}
	logger.Info("Granting privileges",
		"database", db.Spec.DatabaseName,
		"username", username,
		"privileges", privileges)
	if err := dbClient.GrantAllPrivileges(ctx, db.Spec.DatabaseName, username); err != nil {
		return err
	}
	logger.Info("Privileges granted successfully",
		"database", db.Spec.DatabaseName,
		"username", username)

	port, _ := strconv.Atoi(connInfo.Port)

	// Always store credentials in AWS Secrets Manager
	if err := r.storeCredentialsInAWS(ctx, db, username, password, connInfo, port, needsSecretUpdate); err != nil {
		return err
	}

	return nil
}

func (r *DatabaseReconciler) storeCredentialsInAWS(ctx context.Context, db *databasev1alpha1.Database, username, password string, connInfo *database.ConnectionInfo, port int, isMigration bool) error {
	logger := log.FromContext(ctx)

	// Construct database URL
	engine := string(db.Spec.Engine)
	// Normalize engine name for URL scheme
	urlScheme := engine
	if strings.HasPrefix(strings.ToLower(engine), "postgres") {
		urlScheme = "postgresql"
	} else if strings.ToLower(engine) == "mariadb" {
		urlScheme = "mysql" // MariaDB uses mysql:// scheme
	}

	databaseURL := fmt.Sprintf("%s://%s:%s@%s:%d/%s",
		urlScheme,
		url.QueryEscape(username),
		url.QueryEscape(password),
		connInfo.Host,
		port,
		db.Spec.DatabaseName,
	)

	secretValue := &secrets.DatabaseSecret{
		DBHost:      connInfo.Host,
		DBPort:      port,
		DBName:      db.Spec.DatabaseName,
		DBUsername:  username,
		DBPassword:  password,
		DatabaseURL: databaseURL,
		Engine:      engine,
	}

	secretName := getSecretNameOrDefault(db)
	db.Status.ActualSecretName = secretName

	// Determine region: use awsSecretsManager.region if set, otherwise use connectionStringAWSSecretRef.region
	region := r.getRegion(db)
	var regionSource string
	if db.Spec.AWSSecretsManager != nil && db.Spec.AWSSecretsManager.Region != "" {
		regionSource = "spec.awsSecretsManager.region"
	} else if db.Spec.ConnectionStringAWSSecretRef != nil && db.Spec.ConnectionStringAWSSecretRef.Region != "" {
		regionSource = "spec.connectionStringAWSSecretRef.region"
	} else {
		regionSource = "AWS SDK default (environment/instance metadata)"
	}

	// Validate region
	if err := secrets.ValidateRegion(region); err != nil {
		return fmt.Errorf("invalid AWS region: %w", err)
	}

	// Detect region changes
	regionChanged := db.Status.SecretRegion != "" && db.Status.SecretRegion != region
	if regionChanged {
		logger.Info("Region change detected - secret will be created in new region",
			"database", db.Spec.DatabaseName,
			"secretName", secretName,
			"oldRegion", db.Status.SecretRegion,
			"newRegion", region,
			"warning", fmt.Sprintf("Secret may still exist in old region (%s) and should be manually deleted if no longer needed", db.Status.SecretRegion))
	}

	if isMigration {
		logger.Info("Migrating secret to new format in AWS Secrets Manager",
			"database", db.Spec.DatabaseName,
			"secretName", secretName,
			"region", region,
			"regionSource", regionSource)
	} else {
		logger.Info("Storing credentials in AWS Secrets Manager",
			"database", db.Spec.DatabaseName,
			"secretName", secretName,
			"region", region,
			"regionSource", regionSource,
			"username", username)
	}
	awsClient, err := secrets.NewAWSSecretsManagerClient(ctx, region)
	if err != nil {
		return fmt.Errorf("failed to create AWS Secrets Manager client for storing credentials (ensure pod has AWS permissions via IRSA, instance profile, or credentials): %w", err)
	}

	// Get the actual resolved region from the AWS client
	// This is important when region is "" (using AWS SDK default resolution)
	region = awsClient.GetRegion()
	logger.Info("AWS client created with resolved region",
		"database", db.Spec.DatabaseName,
		"resolvedRegion", region,
		"regionSource", regionSource)

	// Check if secret exists in the target region
	exists, err := awsClient.SecretExists(ctx, secretName)
	if err != nil {
		return err
	}

	// If region changed and secret doesn't exist in new region, check old region
	if regionChanged && !exists && db.Status.SecretRegion != "" {
		logger.Info("Checking for secret in old region",
			"secretName", secretName,
			"oldRegion", db.Status.SecretRegion)

		oldRegionClient, err := secrets.NewAWSSecretsManagerClient(ctx, db.Status.SecretRegion)
		if err != nil {
			logger.Error(err, "Failed to create client for old region",
				"oldRegion", db.Status.SecretRegion)
		} else {
			oldExists, err := oldRegionClient.SecretExists(ctx, secretName)
			if err != nil {
				logger.Error(err, "Failed to check secret in old region",
					"oldRegion", db.Status.SecretRegion)
			} else if oldExists {
				logger.Info("Secret exists in old region - will create in new region",
					"secretName", secretName,
					"oldRegion", db.Status.SecretRegion,
					"newRegion", region,
					"action", fmt.Sprintf("Please manually delete secret from %s if no longer needed", db.Status.SecretRegion))
			}
		}
	}

	var secretARN, versionID string
	createSecret := !exists

	if exists {
		if isMigration {
			logger.Info("Updating existing secret with new format (v2) in AWS Secrets Manager",
				"database", db.Spec.DatabaseName,
				"secretName", secretName)
		} else {
			logger.Info("Updating existing secret in AWS Secrets Manager",
				"database", db.Spec.DatabaseName,
				"secretName", secretName)
		}
		versionID, err = awsClient.UpdateSecretWithTemplate(ctx, secretName, secretValue, db.Spec.SecretTemplate)
		if err != nil {
			// Check if secret was deleted externally
			var notFoundErr *secrets.SecretNotFoundError
			if errors.As(err, &notFoundErr) {
				logger.Info("Secret was deleted externally, will create new secret",
					"secretName", secretName)
				createSecret = true
			} else {
				// Check if secret is marked for deletion
				var markedForDeletionErr *secrets.SecretMarkedForDeletionError
				if errors.As(err, &markedForDeletionErr) {
					logger.Info("Secret is marked for deletion in AWS Secrets Manager, will create new secret",
						"secretName", secretName)
					createSecret = true
				} else {
					return err
				}
			}
		}

		if !createSecret {
			secretARN, _ = awsClient.GetSecretARN(ctx, secretName)
			if isMigration {
				logger.Info("Secret migrated successfully to v2 format in AWS Secrets Manager",
					"database", db.Spec.DatabaseName,
					"secretName", secretName,
					"secretARN", secretARN,
					"versionID", versionID,
					"region", region,
					"format", "v2 (DB_HOST, DB_PORT, DB_NAME, DB_USERNAME, DB_PASSWORD, POSTGRES_URL)")
			} else {
				logger.Info("Secret updated successfully in AWS Secrets Manager",
					"database", db.Spec.DatabaseName,
					"secretName", secretName,
					"secretARN", secretARN,
					"versionID", versionID,
					"region", region)
			}
		}
	}

	if createSecret {
		description := "Database credentials for " + db.Spec.DatabaseName
		tags := map[string]string{"ManagedBy": "database-user-operator"}
		if db.Spec.AWSSecretsManager != nil {
			if db.Spec.AWSSecretsManager.Description != "" {
				description = db.Spec.AWSSecretsManager.Description
			}
			for k, v := range db.Spec.AWSSecretsManager.Tags {
				tags[k] = v
			}
		}
		logger.Info("Creating new secret in AWS Secrets Manager",
			"database", db.Spec.DatabaseName,
			"secretName", secretName,
			"description", description)
		secretARN, versionID, err = awsClient.CreateSecretWithTemplate(ctx, secretName, description, secretValue, tags, db.Spec.SecretTemplate)
		if err != nil {
			return err
		}
		logger.Info("Secret created successfully in AWS Secrets Manager",
			"database", db.Spec.DatabaseName,
			"secretName", secretName,
			"secretARN", secretARN,
			"versionID", versionID,
			"region", region)
	}

	// Always update tags to ensure they're in sync with spec
	desiredTags := map[string]string{"ManagedBy": "database-user-operator"}
	if db.Spec.AWSSecretsManager != nil {
		for k, v := range db.Spec.AWSSecretsManager.Tags {
			desiredTags[k] = v
		}
	}

	// Get existing tags to determine what needs to be removed
	existingTags, err := awsClient.GetSecretTags(ctx, secretName)
	if err != nil {
		logger.Error(err, "Failed to get existing secret tags, will still attempt to update tags",
			"secretName", secretName)
		existingTags = map[string]string{} // Continue with empty set
	}

	// Determine tags to remove (exist but not in desired)
	var tagsToRemove []string
	for existingKey := range existingTags {
		if _, desired := desiredTags[existingKey]; !desired {
			tagsToRemove = append(tagsToRemove, existingKey)
		}
	}

	// Remove unwanted tags
	if len(tagsToRemove) > 0 {
		logger.Info("Removing tags from secret in AWS Secrets Manager",
			"secretName", secretName,
			"tagsToRemove", tagsToRemove)
		if err := awsClient.UntagSecret(ctx, secretName, tagsToRemove); err != nil {
			return fmt.Errorf("failed to remove secret tags: %w", err)
		}
	}

	// Add or update desired tags
	logger.Info("Updating secret tags in AWS Secrets Manager",
		"secretName", secretName,
		"desiredTags", desiredTags)
	if err := awsClient.TagSecret(ctx, secretName, desiredTags); err != nil {
		return fmt.Errorf("failed to update secret tags: %w", err)
	}

	// If region changed, delete secret from old region ONLY after successful creation in new region
	// Only delete if we have a valid secretARN (confirming successful creation in new region)
	if regionChanged && db.Status.SecretRegion != "" && secretARN != "" {
		logger.Info("Deleting secret from old region after successful migration",
			"secretName", secretName,
			"oldRegion", db.Status.SecretRegion,
			"newRegion", region,
			"newSecretARN", secretARN)

		oldRegionClient, err := secrets.NewAWSSecretsManagerClient(ctx, db.Status.SecretRegion)
		if err != nil {
			logger.Error(err, "Failed to create AWS client for old region to delete secret",
				"oldRegion", db.Status.SecretRegion,
				"warning", "Secret may still exist in old region and should be manually deleted")
		} else {
			// Use force delete to remove immediately without recovery window
			if err := oldRegionClient.DeleteSecret(ctx, secretName, true); err != nil {
				logger.Error(err, "Failed to delete secret from old region",
					"oldRegion", db.Status.SecretRegion,
					"secretName", secretName,
					"warning", "Secret may still exist in old region and should be manually deleted")
			} else {
				logger.Info("Successfully deleted secret from old region",
					"secretName", secretName,
					"oldRegion", db.Status.SecretRegion)
			}
		}
	} else if regionChanged && db.Status.SecretRegion != "" && secretARN == "" {
		logger.Error(nil, "Region changed but secret not successfully created in new region - keeping secret in old region",
			"oldRegion", db.Status.SecretRegion,
			"newRegion", region,
			"secretName", secretName)
	}

	db.Status.SecretCreated = true
	db.Status.SecretARN = secretARN
	db.Status.SecretVersion = versionID
	db.Status.SecretFormatVersion = "v2"
	db.Status.SecretRegion = region
	db.Status.ConnectionInfo = databasev1alpha1.ConnectionInfo{
		Host:     connInfo.Host,
		Port:     port,
		Database: db.Spec.DatabaseName,
		Username: username,
		Engine:   string(db.Spec.Engine),
	}

	return nil
}

func (r *DatabaseReconciler) reconcileDelete(ctx context.Context, db *databasev1alpha1.Database) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(db, DatabaseFinalizer) {
		return ctrl.Result{}, nil
	}

	// Get RetainOnDelete value with default of true
	retainOnDelete := true
	if db.Spec.RetainOnDelete != nil {
		retainOnDelete = *db.Spec.RetainOnDelete
	}

	logger.Info("Processing deletion",
		"database", db.Spec.DatabaseName,
		"retainOnDelete", retainOnDelete)

	if !retainOnDelete {
		logger.Info("Starting cleanup of database resources (retainOnDelete=false)",
			"database", db.Spec.DatabaseName,
			"username", db.Status.ActualUsername,
			"secretName", db.Status.ActualSecretName)

		// Track what was actually deleted
		databaseDeleted := false
		userDeleted := false
		secretDeleted := false

		// Drop database and user
		connectionString, _ := r.getConnectionString(ctx, db)
		if connectionString != "" {
			if dbClient, err := database.NewClient(string(db.Spec.Engine), connectionString); err == nil {
				defer func() {
					if closeErr := dbClient.Close(); closeErr != nil {
						logger.Error(closeErr, "Failed to close database connection during cleanup")
					}
				}()
				if db.Status.DatabaseCreated {
					logger.Info("Dropping database",
						"database", db.Spec.DatabaseName)
					if err := dbClient.DropDatabase(ctx, db.Spec.DatabaseName); err == nil {
						databaseDeleted = true
						logger.Info("Database dropped successfully",
							"database", db.Spec.DatabaseName)
					} else {
						logger.Error(err, "Failed to drop database",
							"database", db.Spec.DatabaseName)
					}
				}
				if db.Status.UserCreated {
					logger.Info("Dropping user",
						"username", db.Status.ActualUsername)
					if err := dbClient.DropUser(ctx, db.Status.ActualUsername); err == nil {
						userDeleted = true
						logger.Info("User dropped successfully",
							"username", db.Status.ActualUsername)
					} else {
						logger.Error(err, "Failed to drop user",
							"username", db.Status.ActualUsername)
					}
				}
			}
		}

		// Delete credentials from AWS Secrets Manager
		if db.Status.SecretCreated {
			region := r.getRegion(db)

			// Validate region
			if err := secrets.ValidateRegion(region); err != nil {
				logger.Error(err, "Invalid AWS region for secret deletion, skipping",
					"region", region)
			} else {
				awsClient, err := secrets.NewAWSSecretsManagerClient(ctx, region)
				if err != nil {
					logger.Error(err, "Failed to create AWS Secrets Manager client for deletion",
						"secretName", db.Status.ActualSecretName,
						"region", region)
				} else {
					// Get the actual resolved region
					region = awsClient.GetRegion()

					logger.Info("Deleting secret from AWS Secrets Manager",
						"secretName", db.Status.ActualSecretName,
						"secretARN", db.Status.SecretARN,
						"region", region)

					if err := awsClient.DeleteSecret(ctx, db.Status.ActualSecretName, true); err == nil {
						secretDeleted = true
						logger.Info("Secret deleted successfully from AWS Secrets Manager",
							"secretName", db.Status.ActualSecretName,
							"secretARN", db.Status.SecretARN,
							"region", region)
					} else {
						logger.Error(err, "Failed to delete secret from AWS Secrets Manager",
							"secretName", db.Status.ActualSecretName,
							"secretARN", db.Status.SecretARN,
							"region", region)
					}
				}
			}
		}

		logger.Info("Cleanup completed",
			"database", db.Spec.DatabaseName,
			"databaseDeleted", databaseDeleted,
			"userDeleted", userDeleted,
			"secretDeleted", secretDeleted)
	} else {
		logger.Info("Retaining database resources (retainOnDelete=true)",
			"database", db.Spec.DatabaseName,
			"username", db.Status.ActualUsername,
			"secretName", db.Status.ActualSecretName,
			"secretARN", db.Status.SecretARN,
			"databaseRetained", db.Status.DatabaseCreated,
			"userRetained", db.Status.UserCreated,
			"secretRetained", db.Status.SecretCreated)
	}

	controllerutil.RemoveFinalizer(db, DatabaseFinalizer)
	return ctrl.Result{}, r.Update(ctx, db)
}

func (r *DatabaseReconciler) getConnectionString(ctx context.Context, db *databasev1alpha1.Database) (string, error) {
	logger := log.FromContext(ctx)

	// Validate that only one source is configured
	if err := validateConnectionSource(db); err != nil {
		return "", err
	}

	// Check which source is configured
	if db.Spec.ConnectionStringSecretRef != nil {
		logger.Info("Using Kubernetes Secret for admin connection string",
			"database", db.Spec.DatabaseName,
			"secretName", db.Spec.ConnectionStringSecretRef.Name)
		return r.getConnectionStringFromK8sSecret(ctx, db)
	}

	// Must be AWS secret (already validated)
	logger.Info("Using AWS Secrets Manager for admin connection string",
		"database", db.Spec.DatabaseName,
		"secretName", db.Spec.ConnectionStringAWSSecretRef.SecretName,
		"region", db.Spec.ConnectionStringAWSSecretRef.Region)
	return r.getConnectionStringFromAWSSecret(ctx, db)
}

func (r *DatabaseReconciler) getConnectionStringFromK8sSecret(ctx context.Context, db *databasev1alpha1.Database) (string, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Name: db.Spec.ConnectionStringSecretRef.Name, Namespace: db.Namespace}, secret); err != nil {
		return "", err
	}
	key := getSecretKeyOrDefault(db.Spec.ConnectionStringSecretRef)
	connectionString := string(secret.Data[key])
	if connectionString == "" {
		return "", fmt.Errorf("connection string is empty in secret %s key %s", db.Spec.ConnectionStringSecretRef.Name, key)
	}
	return connectionString, nil
}

func (r *DatabaseReconciler) getConnectionStringFromAWSSecret(ctx context.Context, db *databasev1alpha1.Database) (string, error) {
	logger := log.FromContext(ctx)
	awsRef := db.Spec.ConnectionStringAWSSecretRef

	// Validate region
	if err := secrets.ValidateRegion(awsRef.Region); err != nil {
		return "", fmt.Errorf("invalid AWS region for admin connection string: %w", err)
	}

	logger.Info("Creating AWS Secrets Manager client for admin credentials",
		"database", db.Spec.DatabaseName,
		"region", awsRef.Region)
	awsClient, err := secrets.NewAWSSecretsManagerClient(ctx, awsRef.Region)
	if err != nil {
		return "", fmt.Errorf("failed to create AWS Secrets Manager client (ensure pod has AWS permissions): %w", err)
	}

	logger.Info("Retrieving admin connection string from AWS Secrets Manager",
		"database", db.Spec.DatabaseName,
		"secretName", awsRef.SecretName,
		"region", awsRef.Region)
	secretValue, err := awsClient.GetSecretString(ctx, awsRef.SecretName)
	if err != nil {
		return "", fmt.Errorf("failed to get secret '%s' from AWS Secrets Manager (check IAM permissions and secret exists): %w", awsRef.SecretName, err)
	}

	// If a key is specified, parse JSON and extract the key
	key := awsRef.Key
	if key == "" {
		key = "connectionString"
	}

	// Try to parse as JSON
	if strings.Contains(secretValue, "{") {
		var secretData map[string]interface{}
		if err := json.Unmarshal([]byte(secretValue), &secretData); err != nil {
			return "", fmt.Errorf("failed to parse AWS secret as JSON: %w", err)
		}

		value, ok := secretData[key]
		if !ok {
			return "", fmt.Errorf("key %s not found in AWS secret", key)
		}

		connectionString, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("value for key %s in AWS secret is not a string", key)
		}

		return connectionString, nil
	}

	// If not JSON, return the raw value (useful for simple string secrets)
	return secretValue, nil
}

// validateConnectionSource validates that only one connection string source is configured
func validateConnectionSource(db *databasev1alpha1.Database) error {
	if db.Spec.ConnectionStringSecretRef != nil && db.Spec.ConnectionStringAWSSecretRef != nil {
		return fmt.Errorf("both ConnectionStringSecretRef and ConnectionStringAWSSecretRef are specified, only one is allowed")
	}
	if db.Spec.ConnectionStringSecretRef == nil && db.Spec.ConnectionStringAWSSecretRef == nil {
		return fmt.Errorf("neither ConnectionStringSecretRef nor ConnectionStringAWSSecretRef is specified")
	}
	return nil
}

// getSecretKeyOrDefault returns the secret key from the reference, or the default key if not specified
func getSecretKeyOrDefault(ref *databasev1alpha1.SecretKeyReference) string {
	if ref == nil || ref.Key == "" {
		return "connectionString"
	}
	return ref.Key
}

// getUsernameOrDefault returns the username from the spec, or the database name if not specified
func getUsernameOrDefault(db *databasev1alpha1.Database) string {
	if db.Spec.Username != "" {
		return db.Spec.Username
	}
	return db.Spec.DatabaseName
}

// needsReconciliation determines if the database resources need to be reconciled
func needsReconciliation(db *databasev1alpha1.Database) bool {
	const currentSecretFormatVersion = "v2"

	// Need reconciliation if resources aren't created
	if !db.Status.UserCreated || !db.Status.DatabaseCreated || !db.Status.SecretCreated {
		return true
	}

	// Need reconciliation if generation changed (spec was updated)
	if db.Status.ObservedGeneration != db.Generation {
		return true
	}

	// Need reconciliation if secret format needs updating
	if db.Status.SecretFormatVersion != currentSecretFormatVersion {
		return true
	}

	return false
}

// getSecretNameOrDefault returns the secret name from the spec, or generates a default path
// Default format: rds/<engine>/<databaseName>
func getSecretNameOrDefault(db *databasev1alpha1.Database) string {
	if db.Spec.SecretName != "" {
		return db.Spec.SecretName
	}
	return fmt.Sprintf("rds/%s/%s", db.Spec.Engine, db.Spec.DatabaseName)
}

// tagsEqual compares two tag maps and returns true if they are equal
func tagsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// getTagsToRemove returns tags that exist in current but not in desired
func getTagsToRemove(current, desired map[string]string) []string {
	var toRemove []string
	for key := range current {
		if _, exists := desired[key]; !exists {
			toRemove = append(toRemove, key)
		}
	}
	return toRemove
}

// getTagsToAdd returns tags that are in desired but missing or different in current
func getTagsToAdd(current, desired map[string]string) map[string]string {
	toAdd := make(map[string]string)
	for key, desiredValue := range desired {
		currentValue, exists := current[key]
		if !exists || currentValue != desiredValue {
			toAdd[key] = desiredValue
		}
	}
	return toAdd
}

// getRegion determines the AWS region from the Database spec
// Priority: spec.awsSecretsManager.region > spec.connectionStringAWSSecretRef.region > empty (AWS SDK default)
func (r *DatabaseReconciler) getRegion(db *databasev1alpha1.Database) string {
	if db.Spec.AWSSecretsManager != nil && db.Spec.AWSSecretsManager.Region != "" {
		return db.Spec.AWSSecretsManager.Region
	}
	if db.Spec.ConnectionStringAWSSecretRef != nil && db.Spec.ConnectionStringAWSSecretRef.Region != "" {
		return db.Spec.ConnectionStringAWSSecretRef.Region
	}
	return "" // Empty string means use AWS SDK default
}

// isAWSPermissionError checks if an error is an AWS permission/authorization error
func isAWSPermissionError(err error) bool {
	if err == nil {
		return false
	}

	// Check error message for permission-related keywords
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "accessdeniedexception") ||
		strings.Contains(errMsg, "accessdenied") ||
		strings.Contains(errMsg, "access denied") ||
		strings.Contains(errMsg, "not authorized") ||
		strings.Contains(errMsg, "insufficient permissions") ||
		strings.Contains(errMsg, "forbidden") ||
		strings.Contains(errMsg, "unauthorizedoperation")
}

// isAWSResourceNotFoundError checks if an error is an AWS ResourceNotFoundException
func isAWSResourceNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	// Check error message for resource not found keywords
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "resourcenotfoundexception") ||
		strings.Contains(errMsg, "resource not found") ||
		strings.Contains(errMsg, "can't find the specified secret")
}

// normalizeErrorMessage removes dynamic parts from error messages (like AWS RequestIDs)
// to prevent unnecessary status updates that would trigger immediate reconciliation
func normalizeErrorMessage(errMsg string) string {
	// Remove AWS RequestID patterns (e.g., "RequestID: abc-123-def")
	// This prevents status updates from triggering on every reconciliation
	re := regexp.MustCompile(`RequestID: [a-f0-9-]+,?\s?`)
	normalized := re.ReplaceAllString(errMsg, "")

	// Remove any trailing commas or extra spaces
	normalized = strings.TrimRight(normalized, ", ")

	return normalized
}

func (r *DatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Configure custom rate limiter with exponential backoff: 15s, 30s, 60s
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.Database{}).
		WithOptions(controller.Options{
			RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
				15*time.Second, // Base delay: 15 seconds
				60*time.Second, // Max delay: 60 seconds (caps at 60s after 2 retries)
			),
		}).
		Complete(r)
}
