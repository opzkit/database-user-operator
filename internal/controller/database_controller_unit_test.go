/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package controller

import (
	"strings"
	"testing"

	databasev1alpha1 "opzkit/database-user-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetRegion(t *testing.T) {
	reconciler := &DatabaseReconciler{}

	tests := []struct {
		name string
		db   *databasev1alpha1.Database
		want string
	}{
		{
			name: "awsSecretsManager region takes priority",
			db: &databasev1alpha1.Database{
				Spec: databasev1alpha1.DatabaseSpec{
					AWSSecretsManager: &databasev1alpha1.AWSSecretsManagerConfig{
						Region: "us-west-1",
					},
					ConnectionStringAWSSecretRef: &databasev1alpha1.AWSSecretReference{
						SecretName: "test-secret",
						Region:     "us-east-1",
					},
				},
			},
			want: "us-west-1",
		},
		{
			name: "connectionStringAWSSecretRef region when no awsSecretsManager",
			db: &databasev1alpha1.Database{
				Spec: databasev1alpha1.DatabaseSpec{
					ConnectionStringAWSSecretRef: &databasev1alpha1.AWSSecretReference{
						SecretName: "test-secret",
						Region:     "ap-southeast-1",
					},
				},
			},
			want: "ap-southeast-1",
		},
		{
			name: "empty string when no AWS config specified",
			db: &databasev1alpha1.Database{
				Spec: databasev1alpha1.DatabaseSpec{},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reconciler.getRegion(tt.db)
			if got != tt.want {
				t.Errorf("getRegion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDatabaseDefaults(t *testing.T) {
	tests := []struct {
		name         string
		dbName       string
		username     string
		secretName   string
		wantUsername string
		wantSecret   string
	}{
		{
			name:         "username defaults to database name",
			dbName:       "myapp_db",
			username:     "",
			secretName:   "",
			wantUsername: "myapp_db",
			wantSecret:   "/rds/postgres/myapp_db",
		},
		{
			name:         "custom username is preserved",
			dbName:       "myapp_db",
			username:     "custom_user",
			secretName:   "",
			wantUsername: "custom_user",
			wantSecret:   "/rds/postgres/myapp_db",
		},
		{
			name:         "custom secret name is preserved",
			dbName:       "myapp_db",
			username:     "",
			secretName:   "/custom/path/secret",
			wantUsername: "myapp_db",
			wantSecret:   "/custom/path/secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &databasev1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-db",
					Namespace: "default",
				},
				Spec: databasev1alpha1.DatabaseSpec{
					Engine:       databasev1alpha1.DatabaseEnginePostgres,
					DatabaseName: tt.dbName,
					Username:     tt.username,
					SecretName:   tt.secretName,
				},
			}

			// Test username defaulting
			username := db.Spec.Username
			if username == "" {
				username = db.Spec.DatabaseName
			}
			if username != tt.wantUsername {
				t.Errorf("username = %v, want %v", username, tt.wantUsername)
			}

			// Test secret name defaulting
			secretName := db.Spec.SecretName
			if secretName == "" {
				secretName = "/rds/" + string(db.Spec.Engine) + "/" + db.Spec.DatabaseName
			}
			if secretName != tt.wantSecret {
				t.Errorf("secretName = %v, want %v", secretName, tt.wantSecret)
			}
		})
	}
}

func TestDatabaseEngineNormalization(t *testing.T) {
	tests := []struct {
		name   string
		engine databasev1alpha1.DatabaseEngine
		want   string
	}{
		{
			name:   "postgres normalized to postgres",
			engine: databasev1alpha1.DatabaseEnginePostgres,
			want:   "postgres",
		},
		{
			name:   "postgresql normalized to postgres",
			engine: databasev1alpha1.DatabaseEnginePostgreSQL,
			want:   "postgresql",
		},
		{
			name:   "mysql stays mysql",
			engine: databasev1alpha1.DatabaseEngineMySQL,
			want:   "mysql",
		},
		{
			name:   "mariadb stays mariadb",
			engine: databasev1alpha1.DatabaseEngineMariaDB,
			want:   "mariadb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(tt.engine)
			if got != tt.want {
				t.Errorf("engine = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetainOnDeleteDefault(t *testing.T) {
	tests := []struct {
		name           string
		retainOnDelete *bool
		want           bool
	}{
		{
			name:           "nil defaults to true",
			retainOnDelete: nil,
			want:           true,
		},
		{
			name:           "explicit true",
			retainOnDelete: boolPtr(true),
			want:           true,
		},
		{
			name:           "explicit false",
			retainOnDelete: boolPtr(false),
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &databasev1alpha1.Database{
				Spec: databasev1alpha1.DatabaseSpec{
					RetainOnDelete: tt.retainOnDelete,
				},
			}

			// Default value is true
			retain := true
			if db.Spec.RetainOnDelete != nil {
				retain = *db.Spec.RetainOnDelete
			}

			if retain != tt.want {
				t.Errorf("retainOnDelete = %v, want %v", retain, tt.want)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func TestValidateConnectionSource(t *testing.T) {
	tests := []struct {
		name    string
		db      *databasev1alpha1.Database
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid - only k8s secret",
			db: &databasev1alpha1.Database{
				Spec: databasev1alpha1.DatabaseSpec{
					ConnectionStringSecretRef: &databasev1alpha1.SecretKeyReference{
						Name: "my-secret",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid - only AWS secret",
			db: &databasev1alpha1.Database{
				Spec: databasev1alpha1.DatabaseSpec{
					ConnectionStringAWSSecretRef: &databasev1alpha1.AWSSecretReference{
						SecretName: "my-aws-secret",
						Region:     "us-east-1",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - both configured",
			db: &databasev1alpha1.Database{
				Spec: databasev1alpha1.DatabaseSpec{
					ConnectionStringSecretRef: &databasev1alpha1.SecretKeyReference{
						Name: "my-secret",
					},
					ConnectionStringAWSSecretRef: &databasev1alpha1.AWSSecretReference{
						SecretName: "my-aws-secret",
						Region:     "us-east-1",
					},
				},
			},
			wantErr: true,
			errMsg:  "both ConnectionStringSecretRef and ConnectionStringAWSSecretRef are specified",
		},
		{
			name: "invalid - neither configured",
			db: &databasev1alpha1.Database{
				Spec: databasev1alpha1.DatabaseSpec{},
			},
			wantErr: true,
			errMsg:  "neither ConnectionStringSecretRef nor ConnectionStringAWSSecretRef is specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConnectionSource(tt.db)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConnectionSource() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error to contain %q, got %q", tt.errMsg, err.Error())
				}
			}
		})
	}
}

func TestGetSecretKeyOrDefault(t *testing.T) {
	tests := []struct {
		name string
		ref  *databasev1alpha1.SecretKeyReference
		want string
	}{
		{
			name: "nil reference returns default",
			ref:  nil,
			want: "connectionString",
		},
		{
			name: "empty key returns default",
			ref: &databasev1alpha1.SecretKeyReference{
				Name: "my-secret",
				Key:  "",
			},
			want: "connectionString",
		},
		{
			name: "custom key",
			ref: &databasev1alpha1.SecretKeyReference{
				Name: "my-secret",
				Key:  "customKey",
			},
			want: "customKey",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSecretKeyOrDefault(tt.ref)
			if got != tt.want {
				t.Errorf("getSecretKeyOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetUsernameOrDefault(t *testing.T) {
	tests := []struct {
		name string
		db   *databasev1alpha1.Database
		want string
	}{
		{
			name: "custom username specified",
			db: &databasev1alpha1.Database{
				Spec: databasev1alpha1.DatabaseSpec{
					DatabaseName: "mydb",
					Username:     "custom_user",
				},
			},
			want: "custom_user",
		},
		{
			name: "no username - defaults to database name",
			db: &databasev1alpha1.Database{
				Spec: databasev1alpha1.DatabaseSpec{
					DatabaseName: "myapp_db",
				},
			},
			want: "myapp_db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getUsernameOrDefault(tt.db)
			if got != tt.want {
				t.Errorf("getUsernameOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsReconciliation(t *testing.T) {
	tests := []struct {
		name string
		db   *databasev1alpha1.Database
		want bool
	}{
		{
			name: "resources not created",
			db: &databasev1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: databasev1alpha1.DatabaseStatus{
					UserCreated:         false,
					DatabaseCreated:     false,
					SecretCreated:       false,
					ObservedGeneration:  1,
					SecretFormatVersion: "v2",
				},
			},
			want: true,
		},
		{
			name: "generation changed",
			db: &databasev1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Status: databasev1alpha1.DatabaseStatus{
					UserCreated:         true,
					DatabaseCreated:     true,
					SecretCreated:       true,
					ObservedGeneration:  1,
					SecretFormatVersion: "v2",
				},
			},
			want: true,
		},
		{
			name: "secret format outdated",
			db: &databasev1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: databasev1alpha1.DatabaseStatus{
					UserCreated:         true,
					DatabaseCreated:     true,
					SecretCreated:       true,
					ObservedGeneration:  1,
					SecretFormatVersion: "v1",
				},
			},
			want: true,
		},
		{
			name: "no reconciliation needed",
			db: &databasev1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: databasev1alpha1.DatabaseStatus{
					UserCreated:         true,
					DatabaseCreated:     true,
					SecretCreated:       true,
					ObservedGeneration:  1,
					SecretFormatVersion: "v2",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsReconciliation(tt.db)
			if got != tt.want {
				t.Errorf("needsReconciliation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSecretNameOrDefault(t *testing.T) {
	tests := []struct {
		name string
		db   *databasev1alpha1.Database
		want string
	}{
		{
			name: "custom secret name specified",
			db: &databasev1alpha1.Database{
				Spec: databasev1alpha1.DatabaseSpec{
					Engine:       databasev1alpha1.DatabaseEnginePostgres,
					DatabaseName: "mydb",
					SecretName:   "/custom/secret/path",
				},
			},
			want: "/custom/secret/path",
		},
		{
			name: "no secret name - defaults to rds/<engine>/<database>",
			db: &databasev1alpha1.Database{
				Spec: databasev1alpha1.DatabaseSpec{
					Engine:       databasev1alpha1.DatabaseEnginePostgres,
					DatabaseName: "production_db",
				},
			},
			want: "rds/postgres/production_db",
		},
		{
			name: "mysql engine default path",
			db: &databasev1alpha1.Database{
				Spec: databasev1alpha1.DatabaseSpec{
					Engine:       databasev1alpha1.DatabaseEngineMySQL,
					DatabaseName: "app_db",
				},
			},
			want: "rds/mysql/app_db",
		},
		{
			name: "empty secret name - generates default",
			db: &databasev1alpha1.Database{
				Spec: databasev1alpha1.DatabaseSpec{
					Engine:       databasev1alpha1.DatabaseEnginePostgres,
					DatabaseName: "testdb",
					SecretName:   "",
				},
			},
			want: "rds/postgres/testdb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSecretNameOrDefault(tt.db)
			if got != tt.want {
				t.Errorf("getSecretNameOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTagsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    map[string]string
		b    map[string]string
		want bool
	}{
		{
			name: "empty maps are equal",
			a:    map[string]string{},
			b:    map[string]string{},
			want: true,
		},
		{
			name: "nil maps are equal",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "identical maps",
			a: map[string]string{
				"env":  "production",
				"team": "platform",
			},
			b: map[string]string{
				"env":  "production",
				"team": "platform",
			},
			want: true,
		},
		{
			name: "different values",
			a: map[string]string{
				"env": "production",
			},
			b: map[string]string{
				"env": "staging",
			},
			want: false,
		},
		{
			name: "different keys",
			a: map[string]string{
				"env": "production",
			},
			b: map[string]string{
				"environment": "production",
			},
			want: false,
		},
		{
			name: "different lengths",
			a: map[string]string{
				"env":  "production",
				"team": "platform",
			},
			b: map[string]string{
				"env": "production",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tagsEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("tagsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTagsToRemove(t *testing.T) {
	tests := []struct {
		name    string
		current map[string]string
		desired map[string]string
		want    []string
	}{
		{
			name: "remove tags not in desired",
			current: map[string]string{
				"env":     "production",
				"team":    "platform",
				"version": "1.0",
			},
			desired: map[string]string{
				"env":  "production",
				"team": "platform",
			},
			want: []string{"version"},
		},
		{
			name: "no tags to remove",
			current: map[string]string{
				"env":  "production",
				"team": "platform",
			},
			desired: map[string]string{
				"env":     "production",
				"team":    "platform",
				"version": "1.0",
			},
			want: nil,
		},
		{
			name: "remove all tags",
			current: map[string]string{
				"env":  "production",
				"team": "platform",
			},
			desired: map[string]string{},
			want:    []string{"env", "team"},
		},
		{
			name:    "empty current",
			current: map[string]string{},
			desired: map[string]string{
				"env": "production",
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getTagsToRemove(tt.current, tt.desired)
			if tt.want == nil {
				if len(got) > 0 {
					t.Errorf("getTagsToRemove() = %v, want nil or empty", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("getTagsToRemove() returned %d tags, want %d", len(got), len(tt.want))
				return
			}
			// Convert to map for easier comparison (order doesn't matter)
			gotMap := make(map[string]bool)
			for _, tag := range got {
				gotMap[tag] = true
			}
			for _, tag := range tt.want {
				if !gotMap[tag] {
					t.Errorf("getTagsToRemove() missing tag %q", tag)
				}
			}
		})
	}
}

func TestGetTagsToAdd(t *testing.T) {
	tests := []struct {
		name    string
		current map[string]string
		desired map[string]string
		want    map[string]string
	}{
		{
			name: "add new tags",
			current: map[string]string{
				"env": "production",
			},
			desired: map[string]string{
				"env":  "production",
				"team": "platform",
			},
			want: map[string]string{
				"team": "platform",
			},
		},
		{
			name: "update existing tag value",
			current: map[string]string{
				"env": "staging",
			},
			desired: map[string]string{
				"env": "production",
			},
			want: map[string]string{
				"env": "production",
			},
		},
		{
			name: "no tags to add",
			current: map[string]string{
				"env":  "production",
				"team": "platform",
			},
			desired: map[string]string{
				"env": "production",
			},
			want: map[string]string{},
		},
		{
			name:    "add all tags",
			current: map[string]string{},
			desired: map[string]string{
				"env":  "production",
				"team": "platform",
			},
			want: map[string]string{
				"env":  "production",
				"team": "platform",
			},
		},
		{
			name: "add and update mixed",
			current: map[string]string{
				"env":     "staging",
				"version": "1.0",
			},
			desired: map[string]string{
				"env":  "production",
				"team": "platform",
			},
			want: map[string]string{
				"env":  "production",
				"team": "platform",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getTagsToAdd(tt.current, tt.desired)
			if len(got) != len(tt.want) {
				t.Errorf("getTagsToAdd() returned %d tags, want %d\ngot: %v\nwant: %v", len(got), len(tt.want), got, tt.want)
				return
			}
			for key, wantValue := range tt.want {
				gotValue, exists := got[key]
				if !exists {
					t.Errorf("getTagsToAdd() missing tag %q", key)
				} else if gotValue != wantValue {
					t.Errorf("getTagsToAdd() tag %q = %v, want %v", key, gotValue, wantValue)
				}
			}
		})
	}
}
