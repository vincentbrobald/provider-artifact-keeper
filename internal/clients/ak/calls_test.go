// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package ak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

// recorder is a fake server's script and log: it answers every request with
// status (200 when 0) and reply as JSON (no body when nil), and records the
// last request's method, path, raw query and JSON body.
type recorder struct {
	method, path, query string
	body                map[string]any
	reply               any
	status              int
}

// server starts a test server driven by rec, closed when the test ends, and
// returns a client of it (its URL given with a trailing slash).
func (rec *recorder) server(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path, rec.query = r.Method, r.URL.Path, r.URL.RawQuery
		rec.body = nil
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			if err := json.Unmarshal(b, &rec.body); err != nil {
				t.Errorf("body isn't a JSON object: %s", b)
			}
		}
		if rec.status != 0 {
			w.WriteHeader(rec.status)
		}
		if rec.reply != nil {
			_ = json.NewEncoder(w).Encode(rec.reply)
		}
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL+"/", "tok", srv.Client())
}

// TestCalls checks the method, path and body each client call sends.
func TestCalls(t *testing.T) {
	ctx := context.Background()
	q := int64(5)
	name, active := "CI", false
	cases := []struct {
		name         string
		reply        any
		call         func(c *Client) error
		method, path string
		body         map[string]any
	}{
		{"create repository", map[string]any{"id": "r"}, func(c *Client) error {
			_, err := c.CreateRepository(ctx, CreateRepository{Key: "k", Name: "k", Format: "npm", RepoType: "local", QuotaBytes: &q})
			return err
		}, "POST", "/api/v1/repositories", map[string]any{"key": "k", "name": "k", "format": "npm", "repo_type": "local", "allow_anonymous_access": false, "quota_bytes": 5.0}},
		{"update repository", nil, func(c *Client) error {
			return c.UpdateRepository(ctx, "a b", UpdateRepository{Name: &name, CurationDefaultAction: "allow"})
		}, "PATCH", "/api/v1/repositories/a b", map[string]any{"name": "CI", "curation_default_action": "allow"}},
		{"delete repository", nil, func(c *Client) error { return c.DeleteRepository(ctx, "k") }, "DELETE", "/api/v1/repositories/k", nil},
		{"set members", nil, func(c *Client) error {
			return c.SetMembers(ctx, "v", []Member{{Key: "a", Priority: 1}})
		}, "PUT", "/api/v1/repositories/v/members", map[string]any{"members": []any{map[string]any{"member_key": "a", "priority": 1.0}}}},
		{"get permission", map[string]any{"id": "p"}, func(c *Client) error { _, err := c.GetPermission(ctx, "p"); return err }, "GET", "/api/v1/permissions/p", nil},
		{"create permission", map[string]any{"id": "p"}, func(c *Client) error {
			_, err := c.CreatePermission(ctx, Permission{ID: "ignored", PrincipalType: "group", PrincipalID: "g", TargetType: "repository", TargetID: "r", Actions: []string{"read"}})
			return err
		}, "POST", "/api/v1/permissions", map[string]any{"principal_type": "group", "principal_id": "g", "target_type": "repository", "target_id": "r", "actions": []any{"read"}}},
		{"update permission", nil, func(c *Client) error {
			return c.UpdatePermission(ctx, "p", Permission{ID: "ignored", PrincipalType: "user", PrincipalID: "u", TargetType: "repository", TargetID: "r", Actions: []string{"write"}})
		}, "PUT", "/api/v1/permissions/p", map[string]any{"principal_type": "user", "principal_id": "u", "target_type": "repository", "target_id": "r", "actions": []any{"write"}}},
		{"delete permission", nil, func(c *Client) error { return c.DeletePermission(ctx, "p") }, "DELETE", "/api/v1/permissions/p", nil},
		{"get OIDC provider", map[string]any{"id": "o"}, func(c *Client) error { _, err := c.GetOIDCProvider(ctx, "o"); return err }, "GET", "/api/v1/admin/sso/oidc/o", nil},
		{"create OIDC provider", map[string]any{"id": "o"}, func(c *Client) error {
			_, err := c.CreateOIDCProvider(ctx, OIDCProviderRequest{Name: "kc", ClientSecret: "s"})
			return err
		}, "POST", "/api/v1/admin/sso/oidc", nil},
		{"delete OIDC provider", nil, func(c *Client) error { return c.DeleteOIDCProvider(ctx, "o") }, "DELETE", "/api/v1/admin/sso/oidc/o", nil},
		{"get service account", map[string]any{"id": "s"}, func(c *Client) error { _, err := c.GetServiceAccount(ctx, "s"); return err }, "GET", "/api/v1/service-accounts/s", nil},
		{"create service account", map[string]any{"id": "s"}, func(c *Client) error {
			_, err := c.CreateServiceAccount(ctx, "ci", "")
			return err
		}, "POST", "/api/v1/service-accounts", map[string]any{"name": "ci"}},
		{"create service account with a description", map[string]any{"id": "s"}, func(c *Client) error {
			_, err := c.CreateServiceAccount(ctx, "ci", "the CI")
			return err
		}, "POST", "/api/v1/service-accounts", map[string]any{"name": "ci", "description": "the CI"}},
		{"update service account", nil, func(c *Client) error {
			return c.UpdateServiceAccount(ctx, "s", nil, &active)
		}, "PATCH", "/api/v1/service-accounts/s", map[string]any{"display_name": nil, "is_active": false}},
		{"delete service account", nil, func(c *Client) error { return c.DeleteServiceAccount(ctx, "s") }, "DELETE", "/api/v1/service-accounts/s", nil},
		{"list tokens", map[string]any{"items": []any{}}, func(c *Client) error { _, err := c.ListTokens(ctx, "s"); return err }, "GET", "/api/v1/service-accounts/s/tokens", nil},
		{"create token", map[string]any{"id": "t", "token": "v"}, func(c *Client) error {
			_, err := c.CreateToken(ctx, "s", CreateToken{Name: "ci", Scopes: []string{"read:artifacts"}})
			return err
		}, "POST", "/api/v1/service-accounts/s/tokens", map[string]any{"name": "ci", "scopes": []any{"read:artifacts"}}},
		{"revoke token", nil, func(c *Client) error { return c.RevokeToken(ctx, "s", "t") }, "DELETE", "/api/v1/service-accounts/s/tokens/t", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{reply: tc.reply}
			if err := tc.call(rec.server(t)); err != nil {
				t.Fatal(err)
			}
			if rec.method != tc.method || rec.path != tc.path {
				t.Fatalf("sent %s %s, want %s %s", rec.method, rec.path, tc.method, tc.path)
			}
			if tc.body != nil && !jsonEqual(rec.body, tc.body) {
				t.Fatalf("body %v, want %v", rec.body, tc.body)
			}
		})
	}
}

// jsonEqual reports whether a and b encode to the same JSON (maps encode with
// sorted keys).
func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// TestUpdateOIDCProviderReplacesMapping checks that an update always asks for
// the attribute mapping to be replaced.
func TestUpdateOIDCProviderReplacesMapping(t *testing.T) {
	rec := &recorder{}
	c := rec.server(t)
	if err := c.UpdateOIDCProvider(context.Background(), "o", OIDCProviderRequest{Name: "kc"}); err != nil {
		t.Fatal(err)
	}
	if rec.method != "PUT" || rec.path != "/api/v1/admin/sso/oidc/o" || rec.body["attribute_mapping_replace"] != true {
		t.Fatalf("sent %s %s %v", rec.method, rec.path, rec.body)
	}
}

// TestFinds checks that FindPermission, FindOIDCProvider and
// FindServiceAccount return only an exact match, and nil without error when
// there is none.
func TestFinds(t *testing.T) {
	ctx := context.Background()

	rec := &recorder{reply: map[string]any{"items": []any{
		map[string]any{"id": "1", "principal_type": "group", "principal_id": "g", "target_type": "repository", "target_id": "other"},
		map[string]any{"id": "2", "principal_type": "group", "principal_id": "g", "target_type": "repository", "target_id": "r"}}}}
	p, err := rec.server(t).FindPermission(ctx, "group", "g", "repository", "r")
	if err != nil || p == nil || p.ID != "2" {
		t.Fatalf("the exact match, even among others: %+v, %v", p, err)
	}
	if !strings.Contains(rec.query, "principal_id=g") || !strings.Contains(rec.query, "target_id=r") {
		t.Fatalf("the server must filter: %s", rec.query)
	}
	rec.reply = map[string]any{"items": []any{}}
	if p, err = rec.server(t).FindPermission(ctx, "group", "g", "repository", "r"); p != nil || err != nil {
		t.Fatalf("none: %+v, %v", p, err)
	}

	rec = &recorder{reply: []any{map[string]any{"id": "a", "name": "other"}, map[string]any{"id": "b", "name": "keycloak"}}}
	o, err := rec.server(t).FindOIDCProvider(ctx, "keycloak")
	if err != nil || o == nil || o.ID != "b" {
		t.Fatalf("by name: %+v, %v", o, err)
	}
	if o, err = rec.server(t).FindOIDCProvider(ctx, "none"); o != nil || err != nil {
		t.Fatalf("none: %+v, %v", o, err)
	}

	rec = &recorder{reply: map[string]any{"items": []any{map[string]any{"id": "s", "username": "svc-ci"}}}}
	s, err := rec.server(t).FindServiceAccount(ctx, "svc-ci")
	if err != nil || s == nil || s.ID != "s" {
		t.Fatalf("by user name: %+v, %v", s, err)
	}
	if s, err = rec.server(t).FindServiceAccount(ctx, "svc-other"); s != nil || err != nil {
		t.Fatalf("none: %+v, %v", s, err)
	}
}

// TestFindUserPages checks that FindUser reads the next page when the first
// has no exact match, and stops after the last page.
func TestFindUserPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		items := []map[string]any{{"id": "x", "username": "alice2"}}
		if r.URL.Query().Get("page") == "2" {
			items = []map[string]any{{"id": "u", "username": "alice"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "pagination": map[string]any{"total_pages": 2}})
	}))
	defer srv.Close()
	c := New(srv.URL, "t", srv.Client())
	u, err := c.FindUser(context.Background(), "alice")
	if err != nil || u == nil || u.ID != "u" {
		t.Fatalf("exact name on page 2: %+v, %v", u, err)
	}
	if u, err = c.FindUser(context.Background(), "bob"); u != nil || err != nil {
		t.Fatalf("none after the last page: %+v, %v", u, err)
	}
}

// TestAPIErrors checks the error of a non-2xx answer: its classification, the
// body cut to 300 characters and the message.
func TestAPIErrors(t *testing.T) {
	rec := &recorder{status: http.StatusConflict, reply: strings.Repeat("x", 400)}
	err := rec.server(t).DeleteRepository(context.Background(), "k")
	if !IsConflict(err) || IsNotFound(err) {
		t.Fatalf("a 409: %v", err)
	}
	var e *APIError
	if !errors.As(err, &e) || len([]rune(e.Body)) != 300 || !strings.HasPrefix(err.Error(), "DELETE /repositories/k: 409 ") {
		t.Fatalf("the body is cut to 300 characters, the message names the call: %q", err.Error()[:40])
	}
	if !IsConflict(fmt.Errorf("deleting: %w", err)) {
		t.Fatal("a wrapped APIError is still recognised")
	}
	rec = &recorder{status: http.StatusBadRequest, reply: strings.Repeat("é", 400)}
	if err = rec.server(t).DeleteRepository(context.Background(), "k"); !errors.As(err, &e) || len([]rune(e.Body)) != 300 || !utf8.ValidString(e.Body) {
		t.Fatalf("a multi-byte body is cut to 300 whole characters: %d", len([]rune(e.Body)))
	}
	if IsConflict(nil) || IsNotFound(io.EOF) {
		t.Fatal("other errors are neither")
	}
}

// TestDefaultHTTPClient checks that New without an http.Client makes one with
// a timeout.
func TestDefaultHTTPClient(t *testing.T) {
	if c := New("https://ak", "t", nil); c.http == nil || c.http.Timeout == 0 || c.base != "https://ak" {
		t.Fatalf("a client with a timeout: %+v", c)
	}
}
