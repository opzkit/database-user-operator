/*
Copyright 2025 OpzKit

Licensed under the MIT License.
See LICENSE file in the project root for full license information.
*/

package secrets

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CredentialsKey is the data key inside the Kubernetes Secret holding
// the DatabaseSecret JSON blob. We store the entire JSON under a single
// key so the on-the-wire shape matches AWS Secrets Manager and the
// Infisical backend. Apps can decode the JSON directly, or use
// External Secrets / a sidecar to project individual fields.
const CredentialsKey = "credentials"

// DescriptionAnnotation is where the human-readable secret description
// lands on a Kubernetes Secret (Secrets don't have a "description" field).
const DescriptionAnnotation = "database.opzkit.io/description"

// KubernetesBackend stores generated database credentials as Kubernetes
// Secrets in the configured namespace. The full DatabaseSecret is
// JSON-encoded under the single data key "credentials", matching the
// shape of the AWS Secrets Manager backend.
type KubernetesBackend struct {
	client    client.Client
	namespace string
}

// NewKubernetesBackend constructs a KubernetesBackend that creates
// Secrets in `namespace`.
func NewKubernetesBackend(c client.Client, namespace string) *KubernetesBackend {
	return &KubernetesBackend{client: c, namespace: namespace}
}

// Compile-time check that *KubernetesBackend satisfies Backend.
var _ Backend = (*KubernetesBackend)(nil)

// Exists implements Backend.
func (k *KubernetesBackend) Exists(ctx context.Context, name string) (bool, error) {
	var s corev1.Secret
	err := k.client.Get(ctx, k.key(name), &s)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check kubernetes secret existence: %w", err)
	}
	return true, nil
}

// Get implements Backend.
func (k *KubernetesBackend) Get(ctx context.Context, name string) (*DatabaseSecret, error) {
	var s corev1.Secret
	err := k.client.Get(ctx, k.key(name), &s)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, &SecretNotFoundError{SecretName: k.locator(name), Err: err}
		}
		return nil, fmt.Errorf("failed to get kubernetes secret: %w", err)
	}
	raw, ok := s.Data[CredentialsKey]
	if !ok {
		return nil, fmt.Errorf("kubernetes secret %s missing %q key", k.locator(name), CredentialsKey)
	}
	var dbSecret DatabaseSecret
	if err := json.Unmarshal(raw, &dbSecret); err != nil {
		return nil, fmt.Errorf("failed to unmarshal kubernetes secret %s: %w", k.locator(name), err)
	}
	return &dbSecret, nil
}

// Create implements Backend. If the Secret already exists, its data is
// overwritten — same restore-from-soft-deletion semantics the AWS
// implementation provides.
func (k *KubernetesBackend) Create(ctx context.Context, name, description string, secret *DatabaseSecret, tags map[string]string, template string) (string, string, error) {
	payload, err := secret.ToJSONWithTemplate(template)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal database secret: %w", err)
	}

	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: k.namespace,
			Labels:    copyMap(tags),
			Annotations: map[string]string{
				DescriptionAnnotation: description,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			CredentialsKey: payload,
		},
	}

	if err := k.client.Create(ctx, desired); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return "", "", fmt.Errorf("failed to create kubernetes secret: %w", err)
		}
		// Already exists — overwrite (mirrors AWS restore-on-create behaviour).
		var existing corev1.Secret
		if err := k.client.Get(ctx, k.key(name), &existing); err != nil {
			return "", "", fmt.Errorf("failed to get existing kubernetes secret after AlreadyExists: %w", err)
		}
		existing.Labels = copyMap(tags)
		if existing.Annotations == nil {
			existing.Annotations = map[string]string{}
		}
		existing.Annotations[DescriptionAnnotation] = description
		if existing.Data == nil {
			existing.Data = map[string][]byte{}
		}
		existing.Data[CredentialsKey] = payload
		if err := k.client.Update(ctx, &existing); err != nil {
			return "", "", fmt.Errorf("failed to update existing kubernetes secret: %w", err)
		}
		return k.locator(name), existing.ResourceVersion, nil
	}

	return k.locator(name), desired.ResourceVersion, nil
}

// Update implements Backend.
func (k *KubernetesBackend) Update(ctx context.Context, name string, secret *DatabaseSecret, template string) (string, error) {
	payload, err := secret.ToJSONWithTemplate(template)
	if err != nil {
		return "", fmt.Errorf("failed to marshal database secret: %w", err)
	}

	var s corev1.Secret
	if err := k.client.Get(ctx, k.key(name), &s); err != nil {
		if apierrors.IsNotFound(err) {
			return "", &SecretNotFoundError{SecretName: k.locator(name), Err: err}
		}
		return "", fmt.Errorf("failed to get kubernetes secret for update: %w", err)
	}

	if s.Data == nil {
		s.Data = map[string][]byte{}
	}
	s.Data[CredentialsKey] = payload

	if err := k.client.Update(ctx, &s); err != nil {
		return "", fmt.Errorf("failed to update kubernetes secret: %w", err)
	}
	return s.ResourceVersion, nil
}

// Delete implements Backend. forceDelete is ignored — Kubernetes Secrets
// don't have a soft-delete state.
func (k *KubernetesBackend) Delete(ctx context.Context, name string, forceDelete bool) error {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: k.namespace},
	}
	if err := k.client.Delete(ctx, s); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete kubernetes secret: %w", err)
	}
	return nil
}

// Locator implements Backend. Returns "<namespace>/<name>".
func (k *KubernetesBackend) Locator(ctx context.Context, name string) (string, error) {
	return k.locator(name), nil
}

// SyncTags implements Backend by mapping tags onto Secret labels.
// Kubernetes label values have stricter validation than AWS tags
// (max 63 chars, restricted character set); callers must ensure their
// tag values conform — this method does not sanitize.
func (k *KubernetesBackend) SyncTags(ctx context.Context, name string, desired map[string]string) error {
	var s corev1.Secret
	if err := k.client.Get(ctx, k.key(name), &s); err != nil {
		return fmt.Errorf("failed to get kubernetes secret for tag sync: %w", err)
	}
	s.Labels = copyMap(desired)
	if err := k.client.Update(ctx, &s); err != nil {
		return fmt.Errorf("failed to update kubernetes secret labels: %w", err)
	}
	return nil
}

func (k *KubernetesBackend) key(name string) types.NamespacedName {
	return types.NamespacedName{Namespace: k.namespace, Name: name}
}

func (k *KubernetesBackend) locator(name string) string {
	return k.namespace + "/" + name
}

func copyMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
