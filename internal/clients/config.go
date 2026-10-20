// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

// Package clients turns a managed resource's (Cluster)ProviderConfig into an
// Artifact Keeper client.
package clients

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"brobald.be/provider-artifact-keeper/apis/v1beta1"
	"brobald.be/provider-artifact-keeper/internal/clients/ak"
)

// Credentials are the provider's: the server and an API token.
type Credentials struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// Connect resolves the managed resource's provider config, records its use
// and returns a client.
func Connect(ctx context.Context, kube client.Client, mg resource.ModernManaged) (*ak.Client, error) {
	spec, err := resolve(ctx, kube, mg)
	if err != nil {
		return nil, err
	}
	creds, err := credentials(ctx, kube, spec)
	if err != nil {
		return nil, err
	}
	if creds.URL == "" || creds.Token == "" {
		return nil, errors.New("the credentials need url and token")
	}
	return ak.New(creds.URL, creds.Token, nil), nil
}

func resolve(ctx context.Context, kube client.Client, mg resource.ModernManaged) (*v1beta1.ProviderConfigSpec, error) {
	ref := mg.GetProviderConfigReference()
	if ref == nil {
		return nil, errors.New("no providerConfigRef")
	}
	var spec v1beta1.ProviderConfigSpec
	switch ref.Kind {
	case "ClusterProviderConfig":
		pc := &v1beta1.ClusterProviderConfig{}
		if err := kube.Get(ctx, types.NamespacedName{Name: ref.Name}, pc); err != nil {
			return nil, errors.Wrap(err, "cannot get ClusterProviderConfig")
		}
		spec = pc.Spec
	case "ProviderConfig", "":
		pc := &v1beta1.ProviderConfig{}
		if err := kube.Get(ctx, types.NamespacedName{Namespace: mg.GetNamespace(), Name: ref.Name}, pc); err != nil {
			return nil, errors.Wrap(err, "cannot get ProviderConfig")
		}
		spec = pc.Spec
		// A namespaced ProviderConfig's Secret is in its own namespace.
		if spec.Credentials.SecretRef != nil {
			spec.Credentials.SecretRef.Namespace = mg.GetNamespace()
		}
	default:
		return nil, errors.Errorf("unknown provider config kind %q", ref.Kind)
	}
	t := resource.NewProviderConfigUsageTracker(kube, &v1beta1.ProviderConfigUsage{})
	if err := t.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, "cannot track ProviderConfig usage")
	}
	return &spec, nil
}

// credentials: JSON under the selected key, or (the key missing) the flat
// keys url and token of the Secret.
func credentials(ctx context.Context, kube client.Client, spec *v1beta1.ProviderConfigSpec) (Credentials, error) {
	var c Credentials
	sel := spec.Credentials.CommonCredentialSelectors
	if spec.Credentials.Source == xpv2.CredentialsSourceSecret && sel.SecretRef != nil {
		s := &corev1.Secret{}
		if err := kube.Get(ctx, types.NamespacedName{Namespace: sel.SecretRef.Namespace, Name: sel.SecretRef.Name}, s); err != nil {
			return c, errors.Wrap(err, "cannot get the credentials Secret")
		}
		if _, ok := s.Data[sel.SecretRef.Key]; !ok {
			c.URL = strings.TrimSpace(string(s.Data["url"]))
			c.Token = strings.TrimSpace(string(s.Data["token"]))
			return c, nil
		}
	}
	data, err := resource.CommonCredentialExtractor(ctx, spec.Credentials.Source, kube, sel)
	if err != nil {
		return c, errors.Wrap(err, "cannot extract credentials")
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, errors.Wrap(err, "credentials are not JSON with url and token")
	}
	return c, nil
}
