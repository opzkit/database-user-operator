/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseEngine defines the type of database
// +kubebuilder:validation:Enum=postgres;postgresql;mysql;mariadb
type DatabaseEngine string

const (
	DatabaseEnginePostgres   DatabaseEngine = "postgres"
	DatabaseEnginePostgreSQL DatabaseEngine = "postgresql"
	DatabaseEngineMySQL      DatabaseEngine = "mysql"
	DatabaseEngineMariaDB    DatabaseEngine = "mariadb"
)

// DatabaseSpec defines the desired state of Database
type DatabaseSpec struct {
	// Engine specifies the database engine type
	// +kubebuilder:validation:Required
	// +kubebuilder:default=postgres
	Engine DatabaseEngine `json:"engine"`

	// DatabaseName is the name of the database to create
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z][a-z0-9_]*$`
	DatabaseName string `json:"databaseName"`

	// ConnectionStringSecretRef references a Kubernetes Secret containing the admin connection string
	// to the existing database instance. Must have proper permissions to create databases and users.
	// Either ConnectionStringSecretRef or ConnectionStringAWSSecretRef must be specified.
	// Note: Created database credentials will always be stored in AWS Secrets Manager.
	// +optional
	ConnectionStringSecretRef *SecretKeyReference `json:"connectionStringSecretRef,omitempty"`

	// ConnectionStringAWSSecretRef references an AWS Secrets Manager secret containing the admin connection string
	// Either ConnectionStringSecretRef or ConnectionStringAWSSecretRef must be specified.
	// Note: Created database credentials will always be stored in AWS Secrets Manager.
	// +optional
	ConnectionStringAWSSecretRef *AWSSecretReference `json:"connectionStringAWSSecretRef,omitempty"`

	// Username for the database user to be created
	// Defaults to the DatabaseName if not specified
	// +optional
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z][a-z0-9_]*$`
	Username string `json:"username,omitempty"`

	// SecretName is the name/path for storing the created credentials in AWS Secrets Manager
	// Defaults to rds/<engine>/<databaseName>
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// Privileges defines what privileges to grant to the user
	// Defaults to ALL PRIVILEGES on the created database
	// +optional
	Privileges []string `json:"privileges,omitempty"`

	// RetainOnDelete determines whether to retain the database and user when the CR is deleted
	// Defaults to true (retains resources on deletion)
	// +optional
	// +kubebuilder:default=true
	RetainOnDelete *bool `json:"retainOnDelete,omitempty"`

	// AWSSecretsManager contains AWS Secrets Manager specific configuration for storing created credentials
	// All created credentials are stored in AWS Secrets Manager regardless of connection string source
	// +optional
	AWSSecretsManager *AWSSecretsManagerConfig `json:"awsSecretsManager,omitempty"`

	// SecretTemplate is a Go template for customizing the secret structure
	// Available variables: .DBHost, .DBPort, .DBName, .DBUsername, .DBPassword, .DatabaseURL, .Engine
	// If not specified, uses the default template with DB_HOST, DB_PORT, DB_NAME, DB_USERNAME, DB_PASSWORD, and <ENGINE>_URL
	// The template must produce valid JSON
	// +optional
	SecretTemplate string `json:"secretTemplate,omitempty"`
}

// AWSSecretsManagerConfig contains AWS Secrets Manager specific settings
type AWSSecretsManagerConfig struct {
	// Region is the AWS region for Secrets Manager
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=us-east-1;us-east-2;us-west-1;us-west-2;us-gov-west-1;us-gov-east-1;af-south-1;ap-east-1;ap-south-1;ap-south-2;ap-northeast-1;ap-northeast-2;ap-northeast-3;ap-southeast-1;ap-southeast-2;ap-southeast-3;ap-southeast-4;ca-central-1;ca-west-1;eu-central-1;eu-central-2;eu-west-1;eu-west-2;eu-west-3;eu-south-1;eu-south-2;eu-north-1;me-south-1;me-central-1;sa-east-1;cn-north-1;cn-northwest-1;il-central-1
	Region string `json:"region"`

	// Description is the description for the AWS Secrets Manager secret
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are tags to apply to the AWS Secrets Manager secret
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SecretKeyReference references a key in a Kubernetes Secret
type SecretKeyReference struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Key within the secret
	// Defaults to "connectionString"
	// +optional
	Key string `json:"key,omitempty"`
}

// AWSSecretReference references an AWS Secrets Manager secret
type AWSSecretReference struct {
	// SecretName is the name or ARN of the AWS Secrets Manager secret
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`

	// Key within the secret JSON
	// Defaults to "connectionString"
	// +optional
	Key string `json:"key,omitempty"`

	// Region is the AWS region for Secrets Manager
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=us-east-1;us-east-2;us-west-1;us-west-2;us-gov-west-1;us-gov-east-1;af-south-1;ap-east-1;ap-south-1;ap-south-2;ap-northeast-1;ap-northeast-2;ap-northeast-3;ap-southeast-1;ap-southeast-2;ap-southeast-3;ap-southeast-4;ca-central-1;ca-west-1;eu-central-1;eu-central-2;eu-west-1;eu-west-2;eu-west-3;eu-south-1;eu-south-2;eu-north-1;me-south-1;me-central-1;sa-east-1;cn-north-1;cn-northwest-1;il-central-1
	Region string `json:"region"`
}

// DatabaseStatus defines the observed state of Database
type DatabaseStatus struct {
	// Conditions represent the latest available observations of the Database's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Phase represents the current phase of the Database
	// Possible values: Pending, Creating, Ready, Failed, Deleting
	Phase string `json:"phase,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// DatabaseCreated indicates whether the database has been created
	DatabaseCreated bool `json:"databaseCreated,omitempty"`

	// UserCreated indicates whether the user has been created
	UserCreated bool `json:"userCreated,omitempty"`

	// SecretCreated indicates whether the secret has been created
	SecretCreated bool `json:"secretCreated,omitempty"`

	// SecretARN is the ARN of the created AWS Secrets Manager secret (if applicable)
	SecretARN string `json:"secretARN,omitempty"`

	// SecretVersion is the version ID of the secret
	SecretVersion string `json:"secretVersion,omitempty"`

	// SecretFormatVersion tracks the secret structure version (v1=old format, v2=new format with DB_HOST, etc.)
	SecretFormatVersion string `json:"secretFormatVersion,omitempty"`

	// ActualUsername is the actual username that was created
	ActualUsername string `json:"actualUsername,omitempty"`

	// ActualSecretName is the actual secret name that was created
	ActualSecretName string `json:"actualSecretName,omitempty"`

	// SecretRegion is the AWS region where the secret is stored
	SecretRegion string `json:"secretRegion,omitempty"`

	// ConnectionInfo provides non-sensitive connection information
	ConnectionInfo ConnectionInfo `json:"connectionInfo,omitempty"`
}

// ConnectionInfo provides non-sensitive connection information
type ConnectionInfo struct {
	// Host is the database host
	Host string `json:"host,omitempty"`

	// Port is the database port
	Port int `json:"port,omitempty"`

	// Database is the database name
	Database string `json:"database,omitempty"`

	// Username is the database username
	Username string `json:"username,omitempty"`

	// Engine is the database engine
	Engine string `json:"engine,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=db
// +kubebuilder:printcolumn:name="Engine",type=string,JSONPath=`.spec.engine`
// +kubebuilder:printcolumn:name="Database",type=string,JSONPath=`.spec.databaseName`
// +kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.status.actualUsername`
// +kubebuilder:printcolumn:name="SecretName",type=string,JSONPath=`.status.actualSecretName`
// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=`.status.secretRegion`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Database is the Schema for the databases API
type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseSpec   `json:"spec,omitempty"`
	Status DatabaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseList contains a list of Database
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Database `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Database{}, &DatabaseList{})
}
