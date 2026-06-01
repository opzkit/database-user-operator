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

	// ConnectionString sources the admin DSN used to create the database
	// and user. Exactly one of the inner fields (aws, kubernetes, ...)
	// must be set.
	// +kubebuilder:validation:Required
	ConnectionString ConnectionStringSource `json:"connectionString"`

	// Username for the database user to be created
	// Defaults to the DatabaseName if not specified
	// +optional
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z][a-z0-9_]*$`
	Username string `json:"username,omitempty"`

	// SecretName is the name/path used by the chosen SecretBackend for the
	// generated user credentials. For AWS this is the Secrets Manager
	// secret name; for Kubernetes the Secret name; for Infisical the path
	// inside the configured environment. Defaults to rds/<engine>/<databaseName>.
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

	// SecretBackend chooses where to store the generated user credentials.
	// Exactly one of the inner fields (aws, kubernetes, infisical) must
	// be set.
	// +kubebuilder:validation:Required
	SecretBackend SecretBackend `json:"secretBackend"`

	// SecretTemplate is a Go template for customizing the secret structure
	// Available variables: .DBHost, .DBPort, .DBName, .DBUsername, .DBPassword, .DatabaseURL, .Engine
	// If not specified, uses the default template with DB_HOST, DB_PORT, DB_NAME, DB_USERNAME, DB_PASSWORD, and <ENGINE>_URL
	// The template must produce valid JSON
	// +optional
	SecretTemplate string `json:"secretTemplate,omitempty"`
}

// SecretBackend selects where the generated user credentials are stored.
// Exactly one inner field must be set; the controller fails reconciliation
// if zero or multiple are configured.
type SecretBackend struct {
	// AWS stores credentials in AWS Secrets Manager.
	// +optional
	AWS *AWSSecretBackend `json:"aws,omitempty"`

	// Kubernetes stores credentials as a Kubernetes Secret.
	// +optional
	Kubernetes *KubernetesSecretBackend `json:"kubernetes,omitempty"`

	// Infisical stores credentials in Infisical Cloud (or self-hosted)
	// via Universal Auth.
	// +optional
	Infisical *InfisicalSecretBackend `json:"infisical,omitempty"`

	// Scaleway stores credentials in Scaleway Secret Manager.
	// +optional
	Scaleway *ScalewaySecretBackend `json:"scaleway,omitempty"`
}

// AWSSecretBackend contains AWS Secrets Manager configuration.
type AWSSecretBackend struct {
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

// KubernetesSecretBackend stores generated credentials in a Kubernetes
// Secret. The Secret is created in `namespace`; the secret name comes from
// spec.secretName (default rds/<engine>/<db>).
type KubernetesSecretBackend struct {
	// Namespace the Secret is created in. Defaults to the namespace of
	// the Database resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// InfisicalSecretBackend stores generated credentials in Infisical
// (Cloud or self-hosted) via Universal Auth.
//
// The Infisical V3 secrets API requires a project UUID for
// create/update/delete operations, so we expose ProjectID rather than
// the slug used elsewhere (e.g. the ESO ClusterSecretStore). Find the
// UUID in the Infisical UI under Project Settings.
type InfisicalSecretBackend struct {
	// HostAPI is the Infisical API endpoint. Default: https://app.infisical.com
	// +optional
	// +kubebuilder:default="https://app.infisical.com"
	HostAPI string `json:"hostAPI,omitempty"`

	// ProjectID is the Infisical project UUID.
	// +kubebuilder:validation:Required
	ProjectID string `json:"projectID"`

	// Environment is the Infisical environment slug (e.g. dev, staging, prod).
	// +kubebuilder:validation:Required
	Environment string `json:"environment"`

	// SecretsPath is the folder path inside the environment. Default: "/"
	// +optional
	// +kubebuilder:default="/"
	SecretsPath string `json:"secretsPath,omitempty"`

	// AuthSecretRef references a Kubernetes Secret in the same namespace
	// as the Database holding `clientId` and `clientSecret` keys for
	// Infisical Universal Auth.
	// +kubebuilder:validation:Required
	AuthSecretRef KubernetesSecretRef `json:"authSecretRef"`
}

// ScalewaySecretBackend stores generated credentials in Scaleway Secret
// Manager.
//
// Scaleway scopes secrets to a (Region, Project) pair. Authentication uses
// a Scaleway IAM API key (access_key + secret_key) read from a Kubernetes
// Secret in the same namespace as the Database resource. The standard
// Scaleway IAM permission set required is `SecretManagerSecretAccess` at
// Project scope plus `SecretManagerReadOnly` at Org scope (the latter
// covers the list-by-name lookup the controller performs on every
// reconcile).
type ScalewaySecretBackend struct {
	// Region is the Scaleway region for Secret Manager (e.g. fr-par, nl-ams, pl-waw).
	//
	// Optional: when omitted, the controller falls back to the operator
	// pod's SCW_DEFAULT_REGION environment variable. One of the two — this
	// field or the operator env — must resolve at reconcile time.
	// +optional
	// +kubebuilder:validation:Enum=fr-par;nl-ams;pl-waw
	Region string `json:"region,omitempty"`

	// ProjectID is the Scaleway Project UUID owning the secret.
	//
	// Optional: when omitted, the controller falls back to the operator
	// pod's SCW_DEFAULT_PROJECT_ID environment variable. One of the two —
	// this field or the operator env — must resolve at reconcile time.
	// Setting only the operator default scopes every Database CR to the
	// single Project the operator runs against.
	// +optional
	ProjectID string `json:"projectID,omitempty"`

	// Description is the description applied to the Scaleway secret on
	// create/update.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are key/value tags applied to the Scaleway secret. Scaleway
	// stores tags as a flat list of strings; the controller serialises
	// each entry as "key=value" on update.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// AuthSecretRef references a Kubernetes Secret in the same namespace
	// as the Database holding `access_key` and `secret_key` data keys
	// for the Scaleway IAM API key. Same shape as the Secret consumed by
	// the Scaleway provider in External Secrets Operator.
	//
	// Optional: when omitted, the controller falls back to the operator
	// pod's `SCW_ACCESS_KEY` + `SCW_SECRET_KEY` environment variables
	// (mirror of the AWS-backend IRSA / instance-profile pattern). One
	// of the two — per-CR Secret or operator env — must be present at
	// reconcile time.
	//
	// Security note: the env-fallback path uses one operator-pod IAM
	// key for every Database CR cluster-wide. Per-namespace RBAC no
	// longer scopes which Scaleway Project a CR can target via
	// `projectID`. Scope the operator key narrowly (or stay on the
	// per-CR Secret path) when tenant isolation matters.
	// +optional
	AuthSecretRef *KubernetesSecretRef `json:"authSecretRef,omitempty"`
}

// ConnectionStringSource selects where the admin DSN is read from.
// Exactly one inner field must be set.
type ConnectionStringSource struct {
	// AWS reads the connection string from an AWS Secrets Manager secret.
	// +optional
	AWS *AWSConnectionStringRef `json:"aws,omitempty"`

	// Scaleway reads the connection string from a Scaleway Secret Manager
	// secret. Authentication reuses the same IAM API key shape consumed
	// by `secretBackend.scaleway` (a same-namespace Kubernetes Secret
	// with `access_key` and `secret_key` data keys) so a single Secret
	// can back both the admin-DSN lookup and the per-database credential
	// write.
	// +optional
	Scaleway *ScalewayConnectionStringRef `json:"scaleway,omitempty"`

	// Kubernetes reads the connection string from a Kubernetes Secret in
	// the same namespace as the Database.
	// +optional
	Kubernetes *KubernetesConnectionStringRef `json:"kubernetes,omitempty"`
}

// AWSConnectionStringRef points at a key in an AWS Secrets Manager secret.
type AWSConnectionStringRef struct {
	// SecretName is the name or ARN of the AWS Secrets Manager secret
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`

	// Key within the secret JSON. Defaults to "connectionString".
	// +optional
	Key string `json:"key,omitempty"`

	// Region is the AWS region for Secrets Manager
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=us-east-1;us-east-2;us-west-1;us-west-2;us-gov-west-1;us-gov-east-1;af-south-1;ap-east-1;ap-south-1;ap-south-2;ap-northeast-1;ap-northeast-2;ap-northeast-3;ap-southeast-1;ap-southeast-2;ap-southeast-3;ap-southeast-4;ca-central-1;ca-west-1;eu-central-1;eu-central-2;eu-west-1;eu-west-2;eu-west-3;eu-south-1;eu-south-2;eu-north-1;me-south-1;me-central-1;sa-east-1;cn-north-1;cn-northwest-1;il-central-1
	Region string `json:"region"`
}

// KubernetesConnectionStringRef points at a key in a Kubernetes Secret.
type KubernetesConnectionStringRef struct {
	// Name of the secret in the same namespace as the Database.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Key within the secret. Defaults to "connectionString".
	// +optional
	Key string `json:"key,omitempty"`
}

// ScalewayConnectionStringRef points at a Scaleway Secret Manager
// secret holding the admin DSN. Path + Name address the SM secret
// directly (Scaleway stores Path as a folder attribute separate from
// Name — no path-split heuristic). Key selects a JSON property within
// the secret's payload, or the raw payload is returned when the
// payload isn't JSON.
type ScalewayConnectionStringRef struct {
	// Region is the Scaleway region for Secret Manager.
	//
	// Optional: when omitted, the controller falls back to the operator
	// pod's SCW_DEFAULT_REGION environment variable. One of the two — this
	// field or the operator env — must resolve at reconcile time.
	// +optional
	// +kubebuilder:validation:Enum=fr-par;nl-ams;pl-waw
	Region string `json:"region,omitempty"`

	// ProjectID is the Scaleway Project UUID owning the secret.
	//
	// Optional: when omitted, the controller falls back to the operator
	// pod's SCW_DEFAULT_PROJECT_ID environment variable. One of the two —
	// this field or the operator env — must resolve at reconcile time.
	// +optional
	ProjectID string `json:"projectID,omitempty"`

	// Path is the Scaleway Secret Path (folder). Defaults to "/" if
	// omitted. Match the leading-slash form Scaleway stores it as.
	// +optional
	// +kubebuilder:default="/"
	Path string `json:"path,omitempty"`

	// Name is the Scaleway Secret name (leaf). Must match Scaleway's
	// name regex (`^[_a-zA-Z0-9]([-_.a-zA-Z0-9]*[_a-zA-Z0-9])?$`) — no
	// slashes.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Key within the secret JSON. Defaults to "connectionString". If
	// the secret payload is not JSON the Key is ignored and the raw
	// value is returned (matches the AWS backend behaviour).
	// +optional
	Key string `json:"key,omitempty"`

	// AuthSecretRef references a Kubernetes Secret in the same
	// namespace as the Database holding `access_key` and `secret_key`
	// data keys for the Scaleway IAM API key. Same shape as
	// `secretBackend.scaleway.authSecretRef` so a single Secret can
	// back both the admin-DSN read and the per-DB credential write.
	//
	// Optional: when omitted, the controller falls back to the operator
	// pod's `SCW_ACCESS_KEY` + `SCW_SECRET_KEY` environment variables
	// (mirror of the AWS-backend IRSA / instance-profile pattern). One
	// of the two — per-CR Secret or operator env — must be present at
	// reconcile time.
	//
	// Security note: the env-fallback path uses one operator-pod IAM
	// key for every Database CR cluster-wide. Per-namespace RBAC no
	// longer scopes which Scaleway Project a CR can target via
	// `projectID`. Scope the operator key narrowly (or stay on the
	// per-CR Secret path) when tenant isolation matters.
	// +optional
	AuthSecretRef *KubernetesSecretRef `json:"authSecretRef,omitempty"`
}

// KubernetesSecretRef is a generic reference to a Kubernetes Secret in
// the same namespace as the Database resource.
type KubernetesSecretRef struct {
	// Name of the Secret.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
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

	// SecretLocator is the backend-specific identifier for the stored
	// credential secret. For AWS Secrets Manager, this is the ARN
	// (which carries the region). For a Kubernetes Secret it is
	// "namespace/name". For Infisical it is the full project/env/path.
	SecretLocator string `json:"secretLocator,omitempty"`

	// SecretVersion is the version ID of the secret
	SecretVersion string `json:"secretVersion,omitempty"`

	// SecretFormatVersion tracks the secret structure version (v1=old format, v2=new format with DB_HOST, etc.)
	SecretFormatVersion string `json:"secretFormatVersion,omitempty"`

	// ActualUsername is the actual username that was created
	ActualUsername string `json:"actualUsername,omitempty"`

	// ActualSecretName is the actual secret name that was created
	ActualSecretName string `json:"actualSecretName,omitempty"`

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
// +kubebuilder:printcolumn:name="Locator",type=string,priority=1,JSONPath=`.status.secretLocator`
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
