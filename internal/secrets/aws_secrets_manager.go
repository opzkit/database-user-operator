/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// ValidAWSRegions contains all valid AWS regions
var ValidAWSRegions = map[string]bool{
	// US regions
	"us-east-1":     true,
	"us-east-2":     true,
	"us-west-1":     true,
	"us-west-2":     true,
	"us-gov-west-1": true,
	"us-gov-east-1": true,

	// Africa
	"af-south-1": true,

	// Asia Pacific
	"ap-east-1":      true,
	"ap-south-1":     true,
	"ap-south-2":     true,
	"ap-northeast-1": true,
	"ap-northeast-2": true,
	"ap-northeast-3": true,
	"ap-southeast-1": true,
	"ap-southeast-2": true,
	"ap-southeast-3": true,
	"ap-southeast-4": true,

	// Canada
	"ca-central-1": true,
	"ca-west-1":    true,

	// Europe
	"eu-central-1": true,
	"eu-central-2": true,
	"eu-west-1":    true,
	"eu-west-2":    true,
	"eu-west-3":    true,
	"eu-south-1":   true,
	"eu-south-2":   true,
	"eu-north-1":   true,

	// Middle East
	"me-south-1":   true,
	"me-central-1": true,

	// South America
	"sa-east-1": true,

	// China
	"cn-north-1":     true,
	"cn-northwest-1": true,

	// Israel
	"il-central-1": true,
}

// ValidateRegion checks if the provided region is a valid AWS region
// Returns nil if valid or empty (empty allows AWS SDK default resolution)
// Returns error if the region is explicitly provided but invalid
func ValidateRegion(region string) error {
	// Allow empty region - AWS SDK will resolve from environment/config/metadata
	if region == "" {
		return nil
	}

	if !ValidAWSRegions[region] {
		return fmt.Errorf("invalid AWS region: %s", region)
	}

	return nil
}

// AWSSecretsManagerClient wraps AWS Secrets Manager operations
type AWSSecretsManagerClient struct {
	client *secretsmanager.Client
	region string
}

// DatabaseSecret represents the structure of the secret stored in AWS Secrets Manager
type DatabaseSecret struct {
	DBHost     string `json:"DB_HOST"`
	DBPort     int    `json:"DB_PORT"`
	DBName     string `json:"DB_NAME"`
	DBUsername string `json:"DB_USERNAME"`
	DBPassword string `json:"DB_PASSWORD"`
	// Engine-specific URL field (e.g., POSTGRES_URL, MYSQL_URL)
	DatabaseURL string `json:"-"` // Will be set dynamically with the correct engine name
	Engine      string `json:"-"` // Used to determine the URL field name
}

// ToJSON converts the DatabaseSecret to JSON with the engine-specific URL field
func (s *DatabaseSecret) ToJSON() ([]byte, error) {
	return s.ToJSONWithTemplate("")
}

// ToJSONWithTemplate converts the DatabaseSecret to JSON using a custom template
// If tmplStr is empty, uses the default template
func (s *DatabaseSecret) ToJSONWithTemplate(tmplStr string) ([]byte, error) {
	if tmplStr == "" {
		return s.toJSONDefault()
	}

	// Parse and execute the template
	tmpl, err := template.New("secret").Parse(tmplStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse secret template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, s); err != nil {
		return nil, fmt.Errorf("failed to execute secret template: %w", err)
	}

	// Validate that the output is valid JSON
	var jsonCheck interface{}
	if err := json.Unmarshal(buf.Bytes(), &jsonCheck); err != nil {
		return nil, fmt.Errorf("template output is not valid JSON: %w", err)
	}

	return buf.Bytes(), nil
}

// toJSONDefault converts the DatabaseSecret to JSON using the default format
func (s *DatabaseSecret) toJSONDefault() ([]byte, error) {
	// Build map with all fields
	secretMap := map[string]interface{}{
		"DB_HOST":     s.DBHost,
		"DB_PORT":     s.DBPort,
		"DB_NAME":     s.DBName,
		"DB_USERNAME": s.DBUsername,
		"DB_PASSWORD": s.DBPassword,
	}

	// Add engine-specific URL field
	if s.DatabaseURL != "" && s.Engine != "" {
		urlFieldName := strings.ToUpper(s.Engine) + "_URL"
		// Handle "postgresql" -> "POSTGRES_URL"
		if strings.HasPrefix(strings.ToLower(s.Engine), "postgres") {
			urlFieldName = "POSTGRES_URL"
		}
		secretMap[urlFieldName] = s.DatabaseURL
	}

	return json.Marshal(secretMap)
}

// NewAWSSecretsManagerClient creates a new AWS Secrets Manager client
func NewAWSSecretsManagerClient(ctx context.Context, region string) (*AWSSecretsManagerClient, error) {
	var cfg aws.Config
	var err error

	if region != "" {
		cfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(region))
	} else {
		cfg, err = config.LoadDefaultConfig(ctx)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)

	return &AWSSecretsManagerClient{
		client: client,
		region: cfg.Region,
	}, nil
}

// GetRegion returns the AWS region this client is configured for
func (c *AWSSecretsManagerClient) GetRegion() string {
	return c.region
}

// SecretExists checks if a secret exists
func (c *AWSSecretsManagerClient) SecretExists(ctx context.Context, secretName string) (bool, error) {
	_, err := c.client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretName),
	})
	if err != nil {
		// Check if error is ResourceNotFoundException
		var notFoundErr *types.ResourceNotFoundException
		if ok := errors.As(err, &notFoundErr); ok {
			return false, nil
		}
		return false, fmt.Errorf("failed to describe secret: %w", err)
	}

	return true, nil
}

// CreateSecret creates a new secret in AWS Secrets Manager
// If the secret is scheduled for deletion, it will restore it and update the value
func (c *AWSSecretsManagerClient) CreateSecret(ctx context.Context, secretName, description string, secretValue *DatabaseSecret, tags map[string]string) (string, string, error) {
	return c.CreateSecretWithTemplate(ctx, secretName, description, secretValue, tags, "")
}

// CreateSecretWithTemplate creates a new secret in AWS Secrets Manager using a custom template
// If the secret is scheduled for deletion, it will restore it and update the value
func (c *AWSSecretsManagerClient) CreateSecretWithTemplate(ctx context.Context, secretName, description string, secretValue *DatabaseSecret, tags map[string]string, tmpl string) (string, string, error) {
	// Marshal secret to JSON with engine-specific URL field or custom template
	secretJSON, err := secretValue.ToJSONWithTemplate(tmpl)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal secret value: %w", err)
	}

	// Convert tags
	var awsTags []types.Tag
	for key, value := range tags {
		awsTags = append(awsTags, types.Tag{
			Key:   aws.String(key),
			Value: aws.String(value),
		})
	}

	// Create secret
	input := &secretsmanager.CreateSecretInput{
		Name:         aws.String(secretName),
		Description:  aws.String(description),
		SecretString: aws.String(string(secretJSON)),
		Tags:         awsTags,
	}

	output, err := c.client.CreateSecret(ctx, input)
	if err != nil {
		// Check if secret is scheduled for deletion
		var invalidReqErr *types.InvalidRequestException
		if errors.As(err, &invalidReqErr) {
			errMsg := err.Error()
			if strings.Contains(errMsg, "scheduled for deletion") {
				// Secret exists but is scheduled for deletion - restore it
				if err := c.RestoreSecret(ctx, secretName); err != nil {
					return "", "", fmt.Errorf("failed to restore secret scheduled for deletion: %w", err)
				}

				// Update the restored secret with new values
				versionId, err := c.UpdateSecretWithTemplate(ctx, secretName, secretValue, tmpl)
				if err != nil {
					return "", "", fmt.Errorf("failed to update restored secret: %w", err)
				}

				// Update description and tags
				if err := c.UpdateSecretMetadata(ctx, secretName, description); err != nil {
					return "", "", fmt.Errorf("failed to update secret description: %w", err)
				}

				if err := c.TagSecret(ctx, secretName, tags); err != nil {
					return "", "", fmt.Errorf("failed to update secret tags: %w", err)
				}

				// Get the ARN
				arn, err := c.GetSecretARN(ctx, secretName)
				if err != nil {
					return "", "", fmt.Errorf("failed to get secret ARN after restore: %w", err)
				}

				return arn, versionId, nil
			}
		}
		return "", "", fmt.Errorf("failed to create secret: %w", err)
	}

	return aws.ToString(output.ARN), aws.ToString(output.VersionId), nil
}

// UpdateSecret updates an existing secret
// Returns a special error if the secret doesn't exist
func (c *AWSSecretsManagerClient) UpdateSecret(ctx context.Context, secretName string, secretValue *DatabaseSecret) (string, error) {
	return c.UpdateSecretWithTemplate(ctx, secretName, secretValue, "")
}

// UpdateSecretWithTemplate updates an existing secret using a custom template
// Returns a special error if the secret doesn't exist
func (c *AWSSecretsManagerClient) UpdateSecretWithTemplate(ctx context.Context, secretName string, secretValue *DatabaseSecret, tmpl string) (string, error) {
	// Marshal secret to JSON with engine-specific URL field or custom template
	secretJSON, err := secretValue.ToJSONWithTemplate(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to marshal secret value: %w", err)
	}

	// Update secret
	input := &secretsmanager.UpdateSecretInput{
		SecretId:     aws.String(secretName),
		SecretString: aws.String(string(secretJSON)),
	}

	output, err := c.client.UpdateSecret(ctx, input)
	if err != nil {
		// Check if secret doesn't exist
		var notFoundErr *types.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			return "", &SecretNotFoundError{SecretName: secretName, Err: err}
		}
		// Check if secret is marked for deletion
		var invalidReqErr *types.InvalidRequestException
		if errors.As(err, &invalidReqErr) {
			errMsg := err.Error()
			if strings.Contains(errMsg, "marked for deletion") {
				return "", &SecretMarkedForDeletionError{SecretName: secretName, Err: err}
			}
		}
		return "", fmt.Errorf("failed to update secret: %w", err)
	}

	return aws.ToString(output.VersionId), nil
}

// SecretNotFoundError is returned when a secret doesn't exist
type SecretNotFoundError struct {
	SecretName string
	Err        error
}

func (e *SecretNotFoundError) Error() string {
	return fmt.Sprintf("secret %s does not exist", e.SecretName)
}

func (e *SecretNotFoundError) Unwrap() error {
	return e.Err
}

// SecretMarkedForDeletionError is returned when a secret is marked for deletion
type SecretMarkedForDeletionError struct {
	SecretName string
	Err        error
}

func (e *SecretMarkedForDeletionError) Error() string {
	return fmt.Sprintf("secret %s is marked for deletion", e.SecretName)
}

func (e *SecretMarkedForDeletionError) Unwrap() error {
	return e.Err
}

// DeleteSecret deletes a secret from AWS Secrets Manager
// If forceDelete is true, the secret is deleted immediately without recovery window
func (c *AWSSecretsManagerClient) DeleteSecret(ctx context.Context, secretName string, forceDelete bool) error {
	input := &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String(secretName),
		ForceDeleteWithoutRecovery: aws.Bool(forceDelete),
	}

	if !forceDelete {
		// Set recovery window to 7 days (minimum)
		recoveryWindow := int64(7)
		input.RecoveryWindowInDays = &recoveryWindow
	}

	_, err := c.client.DeleteSecret(ctx, input)
	if err != nil {
		// Ignore if secret doesn't exist
		var notFoundErr *types.ResourceNotFoundException
		if ok := errors.As(err, &notFoundErr); ok {
			return nil
		}
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

// RestoreSecret restores a secret that is scheduled for deletion
func (c *AWSSecretsManagerClient) RestoreSecret(ctx context.Context, secretName string) error {
	input := &secretsmanager.RestoreSecretInput{
		SecretId: aws.String(secretName),
	}

	_, err := c.client.RestoreSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to restore secret: %w", err)
	}

	return nil
}

// UpdateSecretMetadata updates the description of a secret
func (c *AWSSecretsManagerClient) UpdateSecretMetadata(ctx context.Context, secretName, description string) error {
	input := &secretsmanager.UpdateSecretInput{
		SecretId:    aws.String(secretName),
		Description: aws.String(description),
	}

	_, err := c.client.UpdateSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update secret metadata: %w", err)
	}

	return nil
}

// GetSecret retrieves a secret value and handles both old and new formats
func (c *AWSSecretsManagerClient) GetSecret(ctx context.Context, secretName string) (*DatabaseSecret, error) {
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	}

	output, err := c.client.GetSecretValue(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret value: %w", err)
	}

	secretString := aws.ToString(output.SecretString)

	// Try new format first
	var secret DatabaseSecret
	if err := json.Unmarshal([]byte(secretString), &secret); err != nil {
		return nil, fmt.Errorf("failed to unmarshal secret value: %w", err)
	}

	// If DBPassword is empty, try to get from old format
	if secret.DBPassword == "" {
		var oldFormat map[string]interface{}
		if err := json.Unmarshal([]byte(secretString), &oldFormat); err == nil {
			if pwd, ok := oldFormat["password"].(string); ok {
				secret.DBPassword = pwd
			}
			if host, ok := oldFormat["host"].(string); ok && secret.DBHost == "" {
				secret.DBHost = host
			}
			if port, ok := oldFormat["port"].(float64); ok && secret.DBPort == 0 {
				secret.DBPort = int(port)
			}
			if dbname, ok := oldFormat["dbname"].(string); ok && secret.DBName == "" {
				secret.DBName = dbname
			}
			if username, ok := oldFormat["username"].(string); ok && secret.DBUsername == "" {
				secret.DBUsername = username
			}
		}
	}

	return &secret, nil
}

// GetSecretString retrieves a secret value as a raw string
func (c *AWSSecretsManagerClient) GetSecretString(ctx context.Context, secretName string) (string, error) {
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	}

	output, err := c.client.GetSecretValue(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to get secret value: %w", err)
	}

	return aws.ToString(output.SecretString), nil
}

// TagSecret adds or updates tags on a secret
func (c *AWSSecretsManagerClient) TagSecret(ctx context.Context, secretName string, tags map[string]string) error {
	var awsTags []types.Tag
	for key, value := range tags {
		awsTags = append(awsTags, types.Tag{
			Key:   aws.String(key),
			Value: aws.String(value),
		})
	}

	input := &secretsmanager.TagResourceInput{
		SecretId: aws.String(secretName),
		Tags:     awsTags,
	}

	_, err := c.client.TagResource(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to tag secret: %w", err)
	}

	return nil
}

// UntagSecret removes tags from a secret
func (c *AWSSecretsManagerClient) UntagSecret(ctx context.Context, secretName string, tagKeys []string) error {
	if len(tagKeys) == 0 {
		return nil
	}

	input := &secretsmanager.UntagResourceInput{
		SecretId: aws.String(secretName),
		TagKeys:  tagKeys,
	}

	_, err := c.client.UntagResource(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to untag secret: %w", err)
	}

	return nil
}

// GetSecretTags retrieves the tags on a secret
func (c *AWSSecretsManagerClient) GetSecretTags(ctx context.Context, secretName string) (map[string]string, error) {
	output, err := c.client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe secret: %w", err)
	}

	tags := make(map[string]string)
	for _, tag := range output.Tags {
		if tag.Key != nil && tag.Value != nil {
			tags[*tag.Key] = *tag.Value
		}
	}

	return tags, nil
}

// GetSecretARN retrieves the ARN of a secret
func (c *AWSSecretsManagerClient) GetSecretARN(ctx context.Context, secretName string) (string, error) {
	output, err := c.client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretName),
	})
	if err != nil {
		return "", fmt.Errorf("failed to describe secret: %w", err)
	}

	return aws.ToString(output.ARN), nil
}
