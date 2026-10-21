// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package clients

import (
	"context"
	"strings"
	"testing"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"brobald.be/provider-artifact-keeper/apis"
	"brobald.be/provider-artifact-keeper/apis/v1alpha1"
	"brobald.be/provider-artifact-keeper/apis/v1beta1"
)

func secret(ns, name string, data map[string]string) *corev1.Secret {
	s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name}, Data: map[string][]byte{}}
	for k, v := range data {
		s.Data[k] = []byte(v)
	}
	return s
}

func secretSource(ns, name, key string) v1beta1.ProviderConfigSpec {
	return v1beta1.ProviderConfigSpec{Credentials: v1beta1.ProviderCredentials{
		Source: xpv2.CredentialsSourceSecret,
		CommonCredentialSelectors: xpv2.CommonCredentialSelectors{SecretRef: &xpv2.SecretKeySelector{
			SecretReference: xpv2.SecretReference{Namespace: ns, Name: name}, Key: key}},
	}}
}

func kube(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := apis.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

func repo(kind, name string) *v1alpha1.Repository {
	r := &v1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Namespace: "team", Name: "r", UID: "u1"}}
	r.SetProviderConfigReference(&xpv2.ProviderConfigReference{Kind: kind, Name: name})
	return r
}

func TestConnect(t *testing.T) {
	ctx := context.Background()
	cpc := &v1beta1.ClusterProviderConfig{ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Spec: secretSource("crossplane-system", "ak", "credentials")}
	pc := &v1beta1.ProviderConfig{ObjectMeta: metav1.ObjectMeta{Namespace: "team", Name: "own"},
		Spec: secretSource("crossplane-system", "ak", "missing-key")}
	k := kube(t, cpc, pc,
		secret("crossplane-system", "ak", map[string]string{"credentials": `{"url": "https://ak", "token": "admin"}`, "url": "https://wrong", "token": "wrong"}),
		secret("team", "ak", map[string]string{"url": " https://ak-team\n", "token": "team\n"}))

	for _, tc := range []struct {
		name, kind, ref, wantURL, wantTok string
	}{
		{"cluster config, JSON key", "ClusterProviderConfig", "default", "https://ak", "admin"},
		{"namespaced config, flat keys of its own namespace", "ProviderConfig", "own", "https://ak-team", "team"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := repo(tc.kind, tc.ref)
			spec, err := resolve(ctx, k, r)
			if err != nil {
				t.Fatal(err)
			}
			c, err := credentials(ctx, k, spec)
			if err != nil || c.URL != tc.wantURL || c.Token != tc.wantTok {
				t.Fatalf("got url %q, the expected token: %t, %v", c.URL, c.Token == tc.wantTok, err)
			}
			if _, err := Connect(ctx, k, r); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConnectErrors(t *testing.T) {
	ctx := context.Background()
	empty := &v1beta1.ClusterProviderConfig{ObjectMeta: metav1.ObjectMeta{Name: "empty"},
		Spec: secretSource("crossplane-system", "empty", "credentials")}
	notJSON := &v1beta1.ClusterProviderConfig{ObjectMeta: metav1.ObjectMeta{Name: "not-json"},
		Spec: secretSource("crossplane-system", "not-json", "credentials")}
	k := kube(t, empty, notJSON,
		secret("crossplane-system", "empty", map[string]string{"credentials": `{"url": "https://ak"}`}),
		secret("crossplane-system", "not-json", map[string]string{"credentials": "url=x"}))

	noRef := &v1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Namespace: "team", Name: "r"}}
	for _, tc := range []struct {
		name string
		mg   *v1alpha1.Repository
		want string
	}{
		{"no reference", noRef, "no providerConfigRef"},
		{"unknown kind", repo("Other", "x"), "unknown provider config kind"},
		{"no kind (the CRD requires one)", repo("", "own"), "unknown provider config kind"},
		{"missing ClusterProviderConfig", repo("ClusterProviderConfig", "none"), "cannot get ClusterProviderConfig"},
		{"missing ProviderConfig", repo("ProviderConfig", "none"), "cannot get ProviderConfig"},
		{"no token", repo("ClusterProviderConfig", "empty"), "need url and token"},
		{"not JSON", repo("ClusterProviderConfig", "not-json"), "not JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Connect(ctx, k, tc.mg); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error with %q, got %v", tc.want, err)
			}
		})
	}
}
