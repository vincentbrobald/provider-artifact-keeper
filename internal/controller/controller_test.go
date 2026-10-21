// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"brobald.be/provider-artifact-keeper/apis"
	"brobald.be/provider-artifact-keeper/apis/v1alpha1"
	"brobald.be/provider-artifact-keeper/internal/clients/ak"
)

// fakeAK is an in-memory Artifact Keeper serving the calls the controllers
// make, enough for their tests; tests also change its maps directly to
// simulate changes made outside the provider.
type fakeAK struct {
	mu      sync.Mutex
	repos   map[string]map[string]any    // key -> repository
	members map[string][]ak.Member       // virtual repository key -> members
	groups  map[string]string            // name -> id
	users   map[string]string            // name -> id
	perms   map[string]ak.Permission     // id -> permission
	sas     map[string]ak.ServiceAccount // id -> service account
	tokens  map[string][]ak.Token        // service account id -> tokens
	oidc    map[string]map[string]any    // id -> provider (as returned)
	secrets map[string]string            // OIDC provider id -> client secret
	n       int                          // the last ID handed out
}

// newFakeAK returns an empty fakeAK.
func newFakeAK() *fakeAK {
	return &fakeAK{repos: map[string]map[string]any{}, members: map[string][]ak.Member{}, groups: map[string]string{},
		users: map[string]string{}, perms: map[string]ak.Permission{}, sas: map[string]ak.ServiceAccount{},
		tokens: map[string][]ak.Token{}, oidc: map[string]map[string]any{}, secrets: map[string]string{}}
}

// oidcFromRequest is what Artifact Keeper returns for a POST or PUT body:
// the secret replaced by has_secret.
func (f *fakeAK) oidcFromRequest(id string, body map[string]any) map[string]any {
	p := map[string]any{"id": id}
	for k, v := range body {
		switch k {
		case "client_secret":
			f.secrets[id] = v.(string)
		case "attribute_mapping_replace", "admin_group":
		default:
			p[k] = v
		}
	}
	p["has_secret"] = f.secrets[id] != ""
	return p
}

// id returns a new UUID-shaped ID (00000000-0000-0000-0000-000000000001,
// ...), so that serverID accepts it.
func (f *fakeAK) id() string {
	f.n++
	return "00000000-0000-0000-0000-" + strings.Repeat("0", 12-len(itoa(f.n))) + itoa(f.n)
}

// itoa returns i in decimal.
func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

// ServeHTTP answers one API call from the fake's state, under its lock.
// Paths are matched without /api/v1, by exact path or by segments. An
// unknown ID answers 404, an unknown call 501. List answers come on a single
// page; users and groups are searched by substring, permissions aren't
// filtered (FindPermission picks the match).
func (f *fakeAK) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := strings.TrimPrefix(r.URL.Path, "/api/v1")
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	out := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	seg := strings.Split(strings.Trim(p, "/"), "/")
	switch {
	case p == "/repositories" && r.Method == http.MethodPost:
		body["id"] = f.id()
		body["curation_default_action"] = "allow"
		f.repos[body["key"].(string)] = body
		out(body)
	case seg[0] == "repositories" && len(seg) == 3 && seg[2] == "members":
		switch r.Method {
		case http.MethodGet:
			rows := []map[string]any{}
			for _, m := range f.members[seg[1]] {
				rows = append(rows, map[string]any{"member_repo_key": m.Key, "priority": m.Priority})
			}
			out(map[string]any{"members": rows})
		case http.MethodPut:
			ms := []ak.Member{}
			for _, m := range body["members"].([]any) {
				x := m.(map[string]any)
				ms = append(ms, ak.Member{Key: x["member_key"].(string), Priority: int(x["priority"].(float64))})
			}
			f.members[seg[1]] = ms
		}
	case p == "/users":
		items := []map[string]any{}
		for n, id := range f.users {
			if strings.Contains(n, r.URL.Query().Get("search")) {
				items = append(items, map[string]any{"id": id, "username": n})
			}
		}
		out(map[string]any{"items": items, "pagination": map[string]any{"total_pages": 1}})
	case p == "/admin/sso/oidc" && r.Method == http.MethodGet:
		items := []map[string]any{}
		for _, x := range f.oidc {
			items = append(items, x)
		}
		out(items)
	case p == "/admin/sso/oidc" && r.Method == http.MethodPost:
		id := f.id()
		f.oidc[id] = f.oidcFromRequest(id, body)
		out(f.oidc[id])
	case seg[0] == "admin" && len(seg) == 4:
		x, ok := f.oidc[seg[3]]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			out(x)
		case http.MethodPut:
			f.oidc[seg[3]] = f.oidcFromRequest(seg[3], body)
		case http.MethodDelete:
			delete(f.oidc, seg[3])
		}
	case p == "/service-accounts" && r.Method == http.MethodGet:
		items := []ak.ServiceAccount{}
		for _, x := range f.sas {
			items = append(items, x)
		}
		out(map[string]any{"items": items})
	case p == "/service-accounts" && r.Method == http.MethodPost:
		sa := ak.ServiceAccount{ID: f.id(), Username: "svc-" + body["name"].(string), IsActive: true}
		f.sas[sa.ID] = sa
		out(sa)
	case seg[0] == "service-accounts" && len(seg) == 2:
		sa, ok := f.sas[seg[1]]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			out(sa)
		case http.MethodPatch:
			if v, ok := body["is_active"].(bool); ok {
				sa.IsActive = v
			}
			f.sas[seg[1]] = sa
		case http.MethodDelete:
			delete(f.sas, seg[1])
			delete(f.tokens, seg[1])
		}
	case seg[0] == "repositories" && len(seg) == 2:
		repo, ok := f.repos[seg[1]]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			out(repo)
		case http.MethodPatch:
			for k, v := range body {
				repo[k] = v
			}
			out(repo)
		case http.MethodDelete:
			delete(f.repos, seg[1])
		}
	case p == "/groups":
		items := []map[string]any{}
		for n, id := range f.groups {
			if strings.Contains(n, r.URL.Query().Get("search")) {
				items = append(items, map[string]any{"id": id, "name": n})
			}
		}
		out(map[string]any{"items": items, "pagination": map[string]any{"total_pages": 1}})
	case p == "/permissions" && r.Method == http.MethodGet:
		items := []ak.Permission{}
		for _, x := range f.perms {
			items = append(items, x)
		}
		out(map[string]any{"items": items})
	case p == "/permissions" && r.Method == http.MethodPost:
		var x ak.Permission
		b, _ := json.Marshal(body)
		_ = json.Unmarshal(b, &x)
		x.ID = f.id()
		f.perms[x.ID] = x
		out(x)
	case seg[0] == "permissions" && len(seg) == 2:
		x, ok := f.perms[seg[1]]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			out(x)
		case http.MethodPut:
			b, _ := json.Marshal(body)
			_ = json.Unmarshal(b, &x)
			x.ID = seg[1]
			f.perms[seg[1]] = x
		case http.MethodDelete:
			delete(f.perms, seg[1])
		}
	case seg[0] == "service-accounts" && len(seg) == 3 && r.Method == http.MethodGet:
		out(map[string]any{"items": f.tokens[seg[1]]})
	case seg[0] == "service-accounts" && len(seg) == 3 && r.Method == http.MethodPost:
		var t ak.Token
		b, _ := json.Marshal(body)
		_ = json.Unmarshal(b, &t)
		t.ID = f.id()
		f.tokens[seg[1]] = append(f.tokens[seg[1]], t)
		out(map[string]any{"id": t.ID, "token": "secret-" + t.ID, "name": t.Name})
	case seg[0] == "service-accounts" && len(seg) == 4 && r.Method == http.MethodDelete:
		ts := f.tokens[seg[1]]
		for i := range ts {
			if ts[i].ID == seg[3] {
				f.tokens[seg[1]] = append(ts[:i], ts[i+1:]...)
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

// setup starts a fakeAK server, closed when the test ends, and returns it, a
// fake Kubernetes client holding objs (with their status subresource) and
// an Artifact Keeper client of the fake.
func setup(t *testing.T, objs ...client.Object) (*fakeAK, client.Client, *ak.Client) {
	t.Helper()
	s := runtime.NewScheme()
	if err := apis.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	f := newFakeAK()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	kube := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).WithStatusSubresource(objs...).Build()
	return f, kube, ak.New(srv.URL, "t", srv.Client())
}

// om returns the metadata of an object name in namespace team-a.
func om(name string) metav1.ObjectMeta { return metav1.ObjectMeta{Name: name, Namespace: "team-a"} }

// TestRepositoryLifecycle takes a local repository through creation, drift
// and its correction, and checks that a type change is refused.
func TestRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	f, kube, c := setup(t)
	e := &repositoryClient{kube: kube, ak: c}
	cr := &v1alpha1.Repository{ObjectMeta: om("a-maven"), Spec: v1alpha1.RepositorySpec{ForProvider: v1alpha1.RepositoryParameters{Format: "maven", Type: "local", Description: "x"}}}
	meta.SetExternalName(cr, "a-maven")

	obs, err := e.Observe(ctx, cr)
	if err != nil || obs.ResourceExists {
		t.Fatalf("a missing repository: %+v, %v", obs, err)
	}
	if _, err := e.Create(ctx, cr); err != nil {
		t.Fatal(err)
	}
	if obs, err = e.Observe(ctx, cr); err != nil || !obs.ResourceExists || !obs.ResourceUpToDate {
		t.Fatalf("after creation: %+v, %v", obs, err)
	}
	f.repos["a-maven"]["description"] = "changed by hand"
	if obs, _ = e.Observe(ctx, cr); obs.ResourceUpToDate {
		t.Fatal("drift must show")
	}
	if _, err := e.Update(ctx, cr); err != nil {
		t.Fatal(err)
	}
	if obs, _ = e.Observe(ctx, cr); !obs.ResourceUpToDate {
		t.Fatal("drift must be corrected")
	}
	cr.Spec.ForProvider.Type = "remote"
	if _, err := e.Observe(ctx, cr); err == nil {
		t.Fatal("a type change must be refused")
	}
}

// TestPermissionWaitsForGroupThenAdopts checks that a permission for a
// missing group is an error, and that once the group exists its existing
// permission is adopted by ID and its actions updated.
func TestPermissionWaitsForGroupThenAdopts(t *testing.T) {
	ctx := context.Background()
	repo := &v1alpha1.Repository{ObjectMeta: om("a-maven")}
	meta.SetExternalName(repo, "a-maven")
	f, kube, c := setup(t, repo)
	f.repos["a-maven"] = map[string]any{"id": "r1", "key": "a-maven", "format": "maven", "repo_type": "local"}
	e := &permissionClient{kube: kube, ak: c}
	cr := &v1alpha1.Permission{ObjectMeta: om("a-maven-team"), Spec: v1alpha1.PermissionSpec{ForProvider: v1alpha1.PermissionParameters{
		Principal: v1alpha1.PermissionPrincipal{Group: "team-a"}, RepositoryRef: &v1alpha1.LocalReference{Name: "a-maven"}, Actions: []string{"read", "write"}}}}

	if _, err := e.Observe(ctx, cr); err == nil || !strings.Contains(err.Error(), "doesn't exist yet") {
		t.Fatalf("the group's absence must be an error that says so: %v", err)
	}
	f.groups["team-a"] = "g1"
	// The fake's own IDs aren't used here: an existing permission with a
	// UUID of its own, stored under that ID.
	f.perms["p0"] = ak.Permission{ID: "11111111-1111-1111-1111-111111111111", PrincipalType: "group", PrincipalID: "g1", TargetType: "repository", TargetID: "r1", Actions: []string{"read"}}
	f.perms["11111111-1111-1111-1111-111111111111"] = f.perms["p0"]
	delete(f.perms, "p0")
	obs, err := e.Observe(ctx, cr)
	if err != nil || !obs.ResourceExists || obs.ResourceUpToDate || !obs.ResourceLateInitialized {
		t.Fatalf("an existing grant must be adopted, out of date: %+v, %v", obs, err)
	}
	if meta.GetExternalName(cr) != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("external name not adopted: %q", meta.GetExternalName(cr))
	}
	if _, err := e.Update(ctx, cr); err != nil {
		t.Fatal(err)
	}
	if obs, _ = e.Observe(ctx, cr); !obs.ResourceUpToDate {
		t.Fatal("actions must be updated")
	}
}

// TestTokenPublishedAndRotated checks that a new token and the user name are
// published, and that a change of scopes makes Create replace the token and
// revoke the old one.
func TestTokenPublishedAndRotated(t *testing.T) {
	ctx := context.Background()
	sa := &v1alpha1.ServiceAccount{ObjectMeta: om("ci")}
	sa.Status.AtProvider.Username = "svc-a-ci"
	meta.SetExternalName(sa, "22222222-2222-2222-2222-222222222222")
	f, kube, c := setup(t, sa)
	e := &tokenClient{kube: kube, ak: c}
	cr := &v1alpha1.Token{ObjectMeta: om("ci"), Spec: v1alpha1.TokenSpec{ForProvider: v1alpha1.TokenParameters{
		ServiceAccountRef: v1alpha1.LocalReference{Name: "ci"}, Name: "ci", Scopes: []string{"read:artifacts"}}}}

	if obs, err := e.Observe(ctx, cr); err != nil || obs.ResourceExists {
		t.Fatalf("no token yet: %+v, %v", obs, err)
	}
	cre, err := e.Create(ctx, cr)
	if err != nil || !strings.HasPrefix(string(cre.ConnectionDetails[KeyToken]), "secret-") || string(cre.ConnectionDetails[KeyUsername]) != "svc-a-ci" {
		t.Fatalf("token and user name must be published: %+v, %v", cre.ConnectionDetails, err)
	}
	if obs, _ := e.Observe(ctx, cr); !obs.ResourceUpToDate {
		t.Fatal("fresh token must be up to date")
	}
	old := meta.GetExternalName(cr)
	cr.Spec.ForProvider.Scopes = []string{"read:artifacts", "write:artifacts"}
	if obs, _ := e.Observe(ctx, cr); obs.ResourceExists {
		t.Fatal("a token with other scopes reads as missing, so that Create replaces it at once")
	}
	if _, err := e.Update(ctx, cr); err != nil || len(f.tokens["22222222-2222-2222-2222-222222222222"]) != 1 {
		t.Fatalf("Update changes nothing: %v", err)
	}
	cre, err = e.Create(ctx, cr)
	if err != nil || meta.GetExternalName(cr) == old || !strings.HasPrefix(string(cre.ConnectionDetails[KeyToken]), "secret-") {
		t.Fatalf("a new token, published: %v", err)
	}
	ts := f.tokens["22222222-2222-2222-2222-222222222222"]
	if len(ts) != 1 || ts[0].ID != meta.GetExternalName(cr) || len(ts[0].Scopes) != 2 {
		t.Fatalf("the old token revoked, the new one with both scopes: %+v", ts)
	}
	if obs, _ := e.Observe(ctx, cr); !obs.ResourceUpToDate {
		t.Fatal("the new token is up to date")
	}
}

// TestOIDCUpToDate checks oidcUpToDate against defaults, scope order, the
// admin group and a missing secret.
func TestOIDCUpToDate(t *testing.T) {
	cr := &v1alpha1.OIDCProvider{Spec: v1alpha1.OIDCProviderSpec{ForProvider: v1alpha1.OIDCProviderParameters{
		Name: "keycloak", IssuerURL: "https://idp/realms/x", ClientID: "c", AdminGroup: "admins"}}}
	want := request(cr, "")
	have := &ak.OIDCProvider{Name: "keycloak", IssuerURL: "https://idp/realms/x", ClientID: "c", HasSecret: true,
		Scopes: []string{"email", "openid", "profile"}, IsEnabled: true, AutoCreateUsers: true, PKCEEnabled: true, MapGroupsToGroups: true,
		AttributeMapping: map[string]any{"groups_claim": "groups", "username_claim": "preferred_username", "admin_group": "admins"}}
	if !oidcUpToDate(have, want) {
		t.Fatal("defaults applied, scopes in another order: up to date")
	}
	have.AttributeMapping["admin_group"] = "others"
	if oidcUpToDate(have, want) {
		t.Fatal("another admin group must show")
	}
	have.AttributeMapping["admin_group"] = "admins"
	have.HasSecret = false
	if oidcUpToDate(have, want) {
		t.Fatal("a provider without its secret must be updated")
	}
}
