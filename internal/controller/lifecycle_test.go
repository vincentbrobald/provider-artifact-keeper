// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"brobald.be/provider-artifact-keeper/apis/v1alpha1"
	"brobald.be/provider-artifact-keeper/apis/v1beta1"
	"brobald.be/provider-artifact-keeper/internal/clients/ak"
)

// deleting marks o as being deleted.
func deleting(o metav1.Object) {
	now := metav1.Now()
	o.SetDeletionTimestamp(&now)
}

// ptr returns a pointer to a copy of v.
func ptr[T any](v T) *T { return &v }

// TestOIDCProviderLifecycle takes an OIDC provider through creation, a
// change of the Secret, adoption by name, deletion, and a Secret without the
// key.
func TestOIDCProviderLifecycle(t *testing.T) {
	ctx := context.Background()
	s := &corev1.Secret{ObjectMeta: om("kc"), Data: map[string][]byte{"secret": []byte("one\n")}}
	f, kube, c := setup(t, s)
	e := &oidcClient{kube: kube, ak: c}
	cr := &v1alpha1.OIDCProvider{ObjectMeta: om("kc"), Spec: v1alpha1.OIDCProviderSpec{ForProvider: v1alpha1.OIDCProviderParameters{
		Name: "keycloak", IssuerURL: "https://idp/realms/x", ClientID: "c",
		ClientSecretSecretRef: v1alpha1.SecretKeySelector{Name: "kc", Key: "secret"}}}}

	if obs, err := e.Observe(ctx, cr); err != nil || obs.ResourceExists {
		t.Fatalf("none of that name yet: %+v, %v", obs, err)
	}
	if _, err := e.Create(ctx, cr); err != nil {
		t.Fatal(err)
	}
	id := meta.GetExternalName(cr)
	if serverID(cr) == "" || f.secrets[id] != "one" || cr.Status.AtProvider.ClientSecretHash != hash("one") {
		t.Fatalf("created with the trimmed secret and its hash recorded: %q", id)
	}
	if obs, err := e.Observe(ctx, cr); err != nil || !obs.ResourceUpToDate || obs.ResourceLateInitialized {
		t.Fatalf("after creation: %+v, %v", obs, err)
	}

	s.Data["secret"] = []byte("two")
	if err := kube.Update(ctx, s); err != nil {
		t.Fatal(err)
	}
	if obs, _ := e.Observe(ctx, cr); obs.ResourceUpToDate {
		t.Fatal("a changed Secret must show")
	}
	if _, err := e.Update(ctx, cr); err != nil {
		t.Fatal(err)
	}
	if obs, _ := e.Observe(ctx, cr); !obs.ResourceUpToDate || f.secrets[id] != "two" {
		t.Fatal("the new secret must be sent and recorded")
	}

	twin := cr.DeepCopy()
	meta.SetExternalName(twin, "")
	twin.Status.AtProvider = v1alpha1.OIDCProviderObservation{}
	obs, err := e.Observe(ctx, twin)
	if err != nil || !obs.ResourceExists || !obs.ResourceLateInitialized || meta.GetExternalName(twin) != id {
		t.Fatalf("adoption by name: %+v, %v", obs, err)
	}
	if obs.ResourceUpToDate {
		t.Fatal("an adopted provider whose secret hash is unknown gets the secret sent again")
	}

	deleting(cr)
	if obs, err := e.Observe(ctx, cr); err != nil || !obs.ResourceUpToDate {
		t.Fatalf("being deleted: no comparison: %+v, %v", obs, err)
	}
	if _, err := e.Delete(ctx, cr); err != nil {
		t.Fatal(err)
	}
	if obs, err := e.Observe(ctx, cr); err != nil || obs.ResourceExists {
		t.Fatalf("deleted: %+v, %v", obs, err)
	}
	if _, err := e.Delete(ctx, cr); err != nil {
		t.Fatalf("deleting again is no error: %v", err)
	}

	s.Data = map[string][]byte{}
	if err := kube.Update(ctx, s); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Create(ctx, twin); err == nil || !strings.Contains(err.Error(), "has no key secret") {
		t.Fatalf("a Secret without the key: %v", err)
	}
}

// TestServiceAccountLifecycle takes a service account through creation,
// deactivation, adoption of svc-<name> and deletion.
func TestServiceAccountLifecycle(t *testing.T) {
	ctx := context.Background()
	f, _, c := setup(t)
	e := &serviceAccountClient{ak: c}
	cr := &v1alpha1.ServiceAccount{ObjectMeta: om("ci"), Spec: v1alpha1.ServiceAccountSpec{ForProvider: v1alpha1.ServiceAccountParameters{Name: "a-ci"}}}

	if obs, err := e.Observe(ctx, cr); err != nil || obs.ResourceExists {
		t.Fatalf("none yet: %+v, %v", obs, err)
	}
	if _, err := e.Create(ctx, cr); err != nil {
		t.Fatal(err)
	}
	id := serverID(cr)
	if obs, err := e.Observe(ctx, cr); err != nil || !obs.ResourceUpToDate || cr.Status.AtProvider.Username != "svc-a-ci" {
		t.Fatalf("after creation: %+v, %v, %+v", obs, err, cr.Status.AtProvider)
	}
	cr.Spec.ForProvider.Active = ptr(false)
	if obs, _ := e.Observe(ctx, cr); obs.ResourceUpToDate {
		t.Fatal("deactivation must show")
	}
	if _, err := e.Update(ctx, cr); err != nil || f.sas[id].IsActive {
		t.Fatalf("deactivated: %v", err)
	}

	twin := &v1alpha1.ServiceAccount{ObjectMeta: om("ci2"), Spec: cr.Spec}
	if obs, err := e.Observe(ctx, twin); err != nil || !obs.ResourceLateInitialized || serverID(twin) != id {
		t.Fatalf("adoption of svc-<name>: %+v, %v", obs, err)
	}

	cr.Spec.ForProvider.Active = nil
	deleting(cr)
	if _, err := e.Delete(ctx, cr); err != nil {
		t.Fatal(err)
	}
	if obs, err := e.Observe(ctx, cr); err != nil || obs.ResourceExists {
		t.Fatalf("deleted: %+v, %v", obs, err)
	}
	if _, err := e.Delete(ctx, cr); err != nil {
		t.Fatalf("deleting again is no error: %v", err)
	}
}

// TestPermissionPrincipalsAndDeletion checks user and service account
// principals, the errors of a missing user, principal or repository, and
// deletion with and without an ID.
func TestPermissionPrincipalsAndDeletion(t *testing.T) {
	ctx := context.Background()
	sa := &v1alpha1.ServiceAccount{ObjectMeta: om("ci")}
	meta.SetExternalName(sa, "22222222-2222-2222-2222-222222222222")
	f, kube, c := setup(t, sa)
	f.repos["a-npm"] = map[string]any{"id": "r1", "key": "a-npm", "format": "npm", "repo_type": "local"}
	f.users["alice"] = "u1"
	e := &permissionClient{kube: kube, ak: c}
	perm := func(p v1alpha1.PermissionPrincipal) *v1alpha1.Permission {
		return &v1alpha1.Permission{ObjectMeta: om("p"), Spec: v1alpha1.PermissionSpec{ForProvider: v1alpha1.PermissionParameters{
			Principal: p, RepositoryKey: "a-npm", Actions: []string{"read"}}}}
	}

	for _, tc := range []struct {
		name          string
		principal     v1alpha1.PermissionPrincipal
		wantType, wID string
	}{
		{"a user", v1alpha1.PermissionPrincipal{User: "alice"}, "user", "u1"},
		{"a service account", v1alpha1.PermissionPrincipal{ServiceAccountRef: &v1alpha1.LocalReference{Name: "ci"}}, "service_account", "22222222-2222-2222-2222-222222222222"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cr := perm(tc.principal)
			if _, err := e.Create(ctx, cr); err != nil {
				t.Fatal(err)
			}
			got := f.perms[serverID(cr)]
			if got.PrincipalType != tc.wantType || got.PrincipalID != tc.wID || got.TargetID != "r1" {
				t.Fatalf("granted %+v", got)
			}
			if obs, err := e.Observe(ctx, cr); err != nil || !obs.ResourceUpToDate || obs.ResourceLateInitialized {
				t.Fatalf("after creation: %+v, %v", obs, err)
			}
			delete(f.users, "alice")
			deleting(cr)
			if obs, err := e.Observe(ctx, cr); err != nil || !obs.ResourceExists {
				t.Fatalf("being deleted: %+v, %v", obs, err)
			}
			if _, err := e.Delete(ctx, cr); err != nil {
				t.Fatal(err)
			}
			if obs, err := e.Observe(ctx, cr); err != nil || obs.ResourceExists {
				t.Fatalf("deleted: %+v, %v", obs, err)
			}
			f.users["alice"] = "u1"
		})
	}

	cr := perm(v1alpha1.PermissionPrincipal{User: "bob"})
	if _, err := e.Observe(ctx, cr); err == nil || !strings.Contains(err.Error(), "user bob doesn't exist") {
		t.Fatalf("a missing user: %v", err)
	}
	if _, err := e.Create(ctx, perm(v1alpha1.PermissionPrincipal{})); err == nil || !strings.Contains(err.Error(), "no principal") {
		t.Fatalf("no principal: %v", err)
	}
	cr = perm(v1alpha1.PermissionPrincipal{User: "alice"})
	cr.Spec.ForProvider.RepositoryKey = ""
	if _, err := e.Observe(ctx, cr); err == nil || !strings.Contains(err.Error(), "neither a repository reference nor a key") {
		t.Fatalf("no repository: %v", err)
	}
	cr = perm(v1alpha1.PermissionPrincipal{User: "alice"})
	deleting(cr)
	if obs, err := e.Observe(ctx, cr); err != nil || obs.ResourceExists {
		t.Fatalf("being deleted, never created: %+v, %v", obs, err)
	}
	if _, err := e.Delete(ctx, cr); err != nil {
		t.Fatalf("deleting one never created: %v", err)
	}
	meta.SetExternalName(cr, "33333333-3333-3333-3333-333333333333")
	if obs, err := e.Observe(ctx, cr); err != nil || obs.ResourceExists {
		t.Fatalf("being deleted, gone: %+v, %v", obs, err)
	}
	cr.SetDeletionTimestamp(nil)
	if obs, err := e.Observe(ctx, cr); err != nil || obs.ResourceExists {
		t.Fatalf("an ID that's gone reads as not existing: %+v, %v", obs, err)
	}
}

// TestVirtualRepository checks a virtual repository's members and their
// order, the quota comparison, a missing member object and deletion.
func TestVirtualRepository(t *testing.T) {
	ctx := context.Background()
	local := &v1alpha1.Repository{ObjectMeta: om("a-npm")}
	meta.SetExternalName(local, "a-npm")
	f, kube, c := setup(t, local)
	e := &repositoryClient{kube: kube, ak: c}
	cr := &v1alpha1.Repository{ObjectMeta: om("a-npm-all"), Spec: v1alpha1.RepositorySpec{ForProvider: v1alpha1.RepositoryParameters{
		Format: "npm", Type: "virtual", Members: []v1alpha1.RepositoryMember{
			{RepositoryRef: &v1alpha1.LocalReference{Name: "a-npm"}}, {Key: "npmjs"}}}}}
	meta.SetExternalName(cr, "a-npm-all")

	if _, err := e.Create(ctx, cr); err != nil {
		t.Fatal(err)
	}
	if m := f.members["a-npm-all"]; len(m) != 2 || m[0] != (ak.Member{Key: "a-npm", Priority: 1}) || m[1] != (ak.Member{Key: "npmjs", Priority: 2}) {
		t.Fatalf("members in order, by priority: %+v", m)
	}
	if obs, err := e.Observe(ctx, cr); err != nil || !obs.ResourceUpToDate || len(cr.Status.AtProvider.Members) != 2 {
		t.Fatalf("after creation: %+v, %v", obs, err)
	}
	f.members["a-npm-all"][0].Priority, f.members["a-npm-all"][1].Priority = 2, 1
	if obs, _ := e.Observe(ctx, cr); obs.ResourceUpToDate {
		t.Fatal("another order is another priority: must show")
	}
	if _, err := e.Update(ctx, cr); err != nil {
		t.Fatal(err)
	}
	if obs, _ := e.Observe(ctx, cr); !obs.ResourceUpToDate {
		t.Fatal("order restored")
	}

	f.repos["a-npm-all"]["quota_bytes"] = 1000
	if obs, _ := e.Observe(ctx, cr); !obs.ResourceUpToDate {
		t.Fatal("unset: the server's quota is left alone")
	}
	cr.Spec.ForProvider.QuotaBytes = ptr(int64(2000))
	if obs, _ := e.Observe(ctx, cr); obs.ResourceUpToDate {
		t.Fatal("set: a different quota must show")
	}

	cr.Spec.ForProvider.Members = append(cr.Spec.ForProvider.Members, v1alpha1.RepositoryMember{RepositoryRef: &v1alpha1.LocalReference{Name: "none"}})
	if _, err := e.Observe(ctx, cr); err == nil || !strings.Contains(err.Error(), "cannot get Repository none") {
		t.Fatalf("a missing member object: %v", err)
	}

	deleting(cr)
	if obs, err := e.Observe(ctx, cr); err != nil || !obs.ResourceUpToDate {
		t.Fatalf("being deleted: %+v, %v", obs, err)
	}
	if _, err := e.Delete(ctx, cr); err != nil {
		t.Fatal(err)
	}
	if obs, err := e.Observe(ctx, cr); err != nil || obs.ResourceExists {
		t.Fatalf("deleted: %+v, %v", obs, err)
	}
	if _, err := e.Delete(ctx, cr); err != nil {
		t.Fatalf("deleting again is no error: %v", err)
	}
}

// TestRepositoryKeyOfAnUncreatedObject checks that a Repository without an
// external name has no key yet.
func TestRepositoryKeyOfAnUncreatedObject(t *testing.T) {
	r := &v1alpha1.Repository{ObjectMeta: om("new")}
	_, kube, _ := setup(t, r)
	if _, err := repositoryKey(context.Background(), kube, "team-a", &v1alpha1.LocalReference{Name: "new"}, ""); err == nil || !strings.Contains(err.Error(), "has no key yet") {
		t.Fatalf("no external name yet: %v", err)
	}
}

// TestTokenOfTheSameNameAndDeletion checks that Create revokes an unknown
// token of the same name, that name and expiresInDays don't replace a token
// but expiry does, and Observe and Delete once the account is gone.
func TestTokenOfTheSameNameAndDeletion(t *testing.T) {
	ctx := context.Background()
	const saID = "22222222-2222-2222-2222-222222222222"
	sa := &v1alpha1.ServiceAccount{ObjectMeta: om("ci")}
	meta.SetExternalName(sa, saID)
	sa.Status.AtProvider.Username = "svc-a-ci"
	repo := &v1alpha1.Repository{ObjectMeta: om("a-npm")}
	meta.SetExternalName(repo, "a-npm")
	f, kube, c := setup(t, sa, repo)
	f.repos["a-npm"] = map[string]any{"id": "r1", "key": "a-npm", "format": "npm", "repo_type": "local"}
	f.repos["b-npm"] = map[string]any{"id": "r2", "key": "b-npm", "format": "npm", "repo_type": "local"}
	f.tokens[saID] = []ak.Token{{ID: "44444444-4444-4444-4444-444444444444", Name: "ci"}}
	e := &tokenClient{kube: kube, ak: c}
	cr := &v1alpha1.Token{ObjectMeta: om("ci"), Spec: v1alpha1.TokenSpec{ForProvider: v1alpha1.TokenParameters{
		ServiceAccountRef: v1alpha1.LocalReference{Name: "ci"}, Name: "ci", Scopes: []string{"write:artifacts"},
		RepositoryRefs: []v1alpha1.LocalReference{{Name: "a-npm"}}, RepositoryKeys: []string{"b-npm"}}}}

	if _, err := e.Create(ctx, cr); err != nil {
		t.Fatal(err)
	}
	ts := f.tokens[saID]
	if len(ts) != 1 || ts[0].ID == "44444444-4444-4444-4444-444444444444" || !sameSet(ts[0].RepositoryIDs, []string{"r1", "r2"}) {
		t.Fatalf("the unknown token of the same name is revoked, a new one made for both repositories: %+v", ts)
	}
	if obs, err := e.Observe(ctx, cr); err != nil || !obs.ResourceUpToDate {
		t.Fatalf("after creation: %+v, %v", obs, err)
	}
	cr.Spec.ForProvider.Name = "renamed"
	cr.Spec.ForProvider.ExpiresInDays = ptr(int64(30))
	if obs, _ := e.Observe(ctx, cr); !obs.ResourceUpToDate {
		t.Fatal("name and expiresInDays apply to the next token only")
	}
	f.tokens[saID][0].IsExpired = true
	if obs, _ := e.Observe(ctx, cr); obs.ResourceExists {
		t.Fatal("an expired token reads as missing, so that Create replaces it")
	}

	if err := kube.Delete(ctx, sa); err != nil {
		t.Fatal(err)
	}
	delete(f.tokens, saID)
	deleting(cr)
	if obs, err := e.Observe(ctx, cr); err != nil || obs.ResourceExists {
		t.Fatalf("account and token gone: %+v, %v", obs, err)
	}
	if _, err := e.Delete(ctx, cr); err != nil {
		t.Fatalf("revoking a token that's gone is no error: %v", err)
	}
	cr.Status.AtProvider.ServiceAccountID = ""
	if obs, err := e.Observe(ctx, cr); err != nil || obs.ResourceExists {
		t.Fatalf("no account to ask, being deleted: gone: %+v, %v", obs, err)
	}
	cr.SetDeletionTimestamp(nil)
	if _, err := e.Observe(ctx, cr); err == nil || !strings.Contains(err.Error(), "cannot get ServiceAccount ci") {
		t.Fatalf("not being deleted, a missing account is an error: %v", err)
	}
}

// TestTokenUserNameOfANewAccount checks that Create asks Artifact Keeper for
// the user name an unobserved ServiceAccount object lacks.
func TestTokenUserNameOfANewAccount(t *testing.T) {
	ctx := context.Background()
	const saID = "22222222-2222-2222-2222-222222222222"
	sa := &v1alpha1.ServiceAccount{ObjectMeta: om("ci")}
	meta.SetExternalName(sa, saID)
	f, kube, c := setup(t, sa)
	f.sas[saID] = ak.ServiceAccount{ID: saID, Username: "svc-a-ci", IsActive: true}
	e := &tokenClient{kube: kube, ak: c}
	cr := &v1alpha1.Token{ObjectMeta: om("ci"), Spec: v1alpha1.TokenSpec{ForProvider: v1alpha1.TokenParameters{
		ServiceAccountRef: v1alpha1.LocalReference{Name: "ci"}, Name: "ci", Scopes: []string{"read:artifacts"}}}}
	cre, err := e.Create(ctx, cr)
	if err != nil || string(cre.ConnectionDetails[KeyUsername]) != "svc-a-ci" {
		t.Fatalf("user name published: %q, %v", cre.ConnectionDetails[KeyUsername], err)
	}
	delete(f.sas, saID)
	if _, err := e.Create(ctx, cr); err == nil || !strings.Contains(err.Error(), "user name") {
		t.Fatalf("no account to ask: %v", err)
	}
}

// TestTokenDeleteRevokes checks that Delete revokes an observed token and
// does nothing for one never observed.
func TestTokenDeleteRevokes(t *testing.T) {
	ctx := context.Background()
	const saID = "22222222-2222-2222-2222-222222222222"
	sa := &v1alpha1.ServiceAccount{ObjectMeta: om("ci")}
	meta.SetExternalName(sa, saID)
	sa.Status.AtProvider.Username = "svc-a-ci"
	f, kube, c := setup(t, sa)
	e := &tokenClient{kube: kube, ak: c}
	cr := &v1alpha1.Token{ObjectMeta: om("ci"), Spec: v1alpha1.TokenSpec{ForProvider: v1alpha1.TokenParameters{
		ServiceAccountRef: v1alpha1.LocalReference{Name: "ci"}, Name: "ci", Scopes: []string{"read:artifacts"}}}}
	if _, err := e.Create(ctx, cr); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Observe(ctx, cr); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Delete(ctx, cr); err != nil || len(f.tokens[saID]) != 0 {
		t.Fatalf("revoked: %+v, %v", f.tokens[saID], err)
	}
	never := &v1alpha1.Token{ObjectMeta: om("never")}
	if _, err := e.Delete(ctx, never); err != nil {
		t.Fatalf("never observed: nothing to revoke: %v", err)
	}
}

// TestClientsRefuseOtherKinds checks that each external client refuses an
// object of another kind (a Token, or a Repository for the Token client).
func TestClientsRefuseOtherKinds(t *testing.T) {
	ctx := context.Background()
	other := &v1alpha1.Token{}
	for _, k := range kinds() {
		e := k.make(nil, nil)
		if k.gvk.Kind == "Token" {
			other := &v1alpha1.Repository{}
			if _, err := e.Observe(ctx, other); err == nil {
				t.Fatal("Token client took a Repository")
			}
			if err := e.Disconnect(ctx); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if _, err := e.Observe(ctx, other); err == nil {
			t.Fatalf("%s client took a Token", k.gvk.Kind)
		}
		if _, err := e.Create(ctx, other); err == nil {
			t.Fatalf("%s client created a Token", k.gvk.Kind)
		}
		if _, err := e.Update(ctx, other); err == nil {
			t.Fatalf("%s client updated a Token", k.gvk.Kind)
		}
		if _, err := e.Delete(ctx, other); err == nil {
			t.Fatalf("%s client deleted a Token", k.gvk.Kind)
		}
		if err := e.Disconnect(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

// TestKinds checks the list of kinds and which have server-assigned IDs.
func TestKinds(t *testing.T) {
	want := map[string]bool{"Repository": false, "Permission": true, "OIDCProvider": true, "ServiceAccount": true, "Token": true}
	ks := kinds()
	if len(ks) != len(want) {
		t.Fatalf("%d kinds", len(ks))
	}
	for _, k := range ks {
		if s, ok := want[k.gvk.Kind]; !ok || s != k.serverID {
			t.Fatalf("%s: serverID %t", k.gvk.Kind, k.serverID)
		}
		if k.obj.GetObjectKind() == nil {
			t.Fatal("no object")
		}
	}
}

// TestDefaults checks displayName and orDefault.
func TestDefaults(t *testing.T) {
	r := &v1alpha1.Repository{ObjectMeta: om("k")}
	meta.SetExternalName(r, "key")
	if displayName(r) != "key" {
		t.Fatal("the key when no name is set")
	}
	r.Spec.ForProvider.Name = "Shown"
	if displayName(r) != "Shown" || orDefault("x", "d") != "x" || orDefault("", "d") != "d" {
		t.Fatal("a set value wins over the default")
	}
}

// TestConnectorNeedsProviderConfig checks that Connect fails without a
// provider config and succeeds with a ClusterProviderConfig and its Secret.
func TestConnectorNeedsProviderConfig(t *testing.T) {
	_, kube, _ := setup(t)
	conn := &connector{kube: kube, make: kinds()[0].make}
	if _, err := conn.Connect(context.Background(), &v1alpha1.Repository{ObjectMeta: om("r")}); err == nil {
		t.Fatal("connected without a provider config")
	}
	cpc := &v1beta1.ClusterProviderConfig{ObjectMeta: metav1.ObjectMeta{Name: "default"}, Spec: v1beta1.ProviderConfigSpec{
		Credentials: v1beta1.ProviderCredentials{Source: xpv2.CredentialsSourceSecret, CommonCredentialSelectors: xpv2.CommonCredentialSelectors{
			SecretRef: &xpv2.SecretKeySelector{SecretReference: xpv2.SecretReference{Namespace: "ns", Name: "ak"}, Key: "credentials"}}}}}
	s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ak"}, Data: map[string][]byte{"url": []byte("https://ak"), "token": []byte("t")}}
	_, kube, _ = setup(t, cpc, s)
	conn = &connector{kube: kube, make: kinds()[0].make}
	r := &v1alpha1.Repository{ObjectMeta: om("r")}
	r.SetUID("u1")
	r.SetProviderConfigReference(&xpv2.ProviderConfigReference{Kind: "ClusterProviderConfig", Name: "default"})
	if e, err := conn.Connect(context.Background(), r); err != nil || e == nil {
		t.Fatalf("a client from the ClusterProviderConfig: %v", err)
	}
}

// A Token deleted before any Observe is still revoked: Create records the
// account's ID, and without one Delete asks the ServiceAccount object.
func TestTokenDeletedBeforeObserve(t *testing.T) {
	ctx := context.Background()
	const saID = "22222222-2222-2222-2222-222222222222"
	sa := &v1alpha1.ServiceAccount{ObjectMeta: om("ci")}
	meta.SetExternalName(sa, saID)
	sa.Status.AtProvider.Username = "svc-a-ci"
	f, kube, c := setup(t, sa)
	e := &tokenClient{kube: kube, ak: c}
	cr := &v1alpha1.Token{ObjectMeta: om("ci"), Spec: v1alpha1.TokenSpec{ForProvider: v1alpha1.TokenParameters{
		ServiceAccountRef: v1alpha1.LocalReference{Name: "ci"}, Name: "ci", Scopes: []string{"read:artifacts"}}}}

	if _, err := e.Create(ctx, cr); err != nil {
		t.Fatal(err)
	}
	if cr.Status.AtProvider.ServiceAccountID != saID {
		t.Fatalf("Create records the account: %q", cr.Status.AtProvider.ServiceAccountID)
	}
	if _, err := e.Delete(ctx, cr); err != nil || len(f.tokens[saID]) != 0 {
		t.Fatalf("revoked without an Observe: %+v, %v", f.tokens[saID], err)
	}

	// Made by an older version: no account ID in status.
	if _, err := e.Create(ctx, cr); err != nil {
		t.Fatal(err)
	}
	cr.Status.AtProvider.ServiceAccountID = ""
	if _, err := e.Delete(ctx, cr); err != nil || len(f.tokens[saID]) != 0 {
		t.Fatalf("revoked through the ServiceAccount object: %+v, %v", f.tokens[saID], err)
	}

	// The account object gone too: nothing to revoke, no error.
	if _, err := e.Create(ctx, cr); err != nil {
		t.Fatal(err)
	}
	cr.Status.AtProvider.ServiceAccountID = ""
	if err := kube.Delete(ctx, sa); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Delete(ctx, cr); err != nil {
		t.Fatalf("no account left: %v", err)
	}
}
