/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import "testing"

func TestAWSRegionFromARN(t *testing.T) {
	tests := []struct {
		name string
		arn  string
		want string
	}{
		{
			name: "secrets manager ARN",
			arn:  "arn:aws:secretsmanager:eu-west-1:123456789012:secret:rds/postgres/myapp-AbCdEf",
			want: "eu-west-1",
		},
		{
			name: "us-gov ARN",
			arn:  "arn:aws-us-gov:secretsmanager:us-gov-west-1:123456789012:secret:foo-AbCdEf",
			want: "us-gov-west-1",
		},
		{
			name: "non-AWS locator (kubernetes namespace/name)",
			arn:  "default/myapp-credentials",
			want: "",
		},
		{
			name: "empty string",
			arn:  "",
			want: "",
		},
		{
			name: "malformed ARN with too few parts",
			arn:  "arn:aws:secretsmanager",
			want: "",
		},
		{
			name: "non-arn prefix",
			arn:  "not-an-arn:aws:secretsmanager:eu-west-1:123:secret:foo",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AWSRegionFromARN(tt.arn)
			if got != tt.want {
				t.Errorf("AWSRegionFromARN(%q) = %q, want %q", tt.arn, got, tt.want)
			}
		})
	}
}
