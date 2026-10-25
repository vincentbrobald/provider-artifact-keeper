// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

// Package ak is a small client of Artifact Keeper's REST API (/api/v1): the
// calls the provider's controllers make, typed after its OpenAPI document
// (GET /api/v1/openapi.json on a server with ENABLE_SWAGGER=true).
package ak

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Client calls one Artifact Keeper server with one API token.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// New returns a client of the server at baseURL (https://host, no path; a
// trailing slash is dropped) that authenticates with token. A nil hc means an
// http.Client with a 30-second timeout.
func New(baseURL, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), token: token, http: hc}
}

// APIError is an answer outside 2xx: the call, the status code and the start
// of the response body.
type APIError struct {
	Method, Path string
	Status       int
	Body         string
}

// Error returns "METHOD /path: status body", the path without /api/v1.
func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: %d %s", e.Method, e.Path, e.Status, e.Body)
}

// IsNotFound reports whether err is an APIError with status 404.
func IsNotFound(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

// IsConflict reports whether err is an APIError with status 409.
func IsConflict(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status == http.StatusConflict
}

// do sends method to /api/v1+path with the bearer token, in (if not nil) as
// the JSON body, and decodes the JSON answer into out (if not nil and the
// answer isn't empty). path must already be escaped. A status outside 2xx
// returns an *APIError; transport and decoding errors are returned as they
// are.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/api/v1"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close() //nolint:errcheck // read-only
	// At most 1 MiB of answer is read; a read error leaves what was read.
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		// The error keeps the body, trimmed and cut to 300 characters.
		msg := strings.TrimSpace(string(b))
		if r := []rune(msg); len(r) > 300 {
			msg = string(r[:300])
		}
		return &APIError{Method: method, Path: path, Status: res.StatusCode, Body: msg}
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}

// pagination is the paging block of a paged list answer.
type pagination struct {
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
}

// Repository is a repository as Artifact Keeper returns it.
type Repository struct {
	ID                    string  `json:"id"`
	Key                   string  `json:"key"`
	Name                  string  `json:"name"`
	Description           *string `json:"description"`
	Format                string  `json:"format"`
	RepoType              string  `json:"repo_type"`
	UpstreamURL           *string `json:"upstream_url"`
	AllowAnonymousAccess  bool    `json:"allow_anonymous_access"`
	QuotaBytes            *int64  `json:"quota_bytes"`
	StorageUsedBytes      int64   `json:"storage_used_bytes"`
	CurationDefaultAction string  `json:"curation_default_action"`
}

// CreateRepository is the body of a repository creation.
type CreateRepository struct {
	Key                  string `json:"key"`
	Name                 string `json:"name"`
	Description          string `json:"description,omitempty"`
	Format               string `json:"format"`
	RepoType             string `json:"repo_type"`
	UpstreamURL          string `json:"upstream_url,omitempty"`
	AllowAnonymousAccess bool   `json:"allow_anonymous_access"`
	QuotaBytes           *int64 `json:"quota_bytes,omitempty"`
}

// UpdateRepository is the body of a repository update: nil fields are left
// out and stay as they are. CurationDefaultAction is required by the API:
// send the current one.
type UpdateRepository struct {
	Name                  *string `json:"name,omitempty"`
	Description           *string `json:"description,omitempty"`
	AllowAnonymousAccess  *bool   `json:"allow_anonymous_access,omitempty"`
	QuotaBytes            *int64  `json:"quota_bytes,omitempty"`
	CurationDefaultAction string  `json:"curation_default_action"`
}

// Member is a member of a virtual repository; the lowest priority is
// searched first.
type Member struct {
	Key      string `json:"member_key"`
	Priority int    `json:"priority"`
}

// memberRow is a member as the members list returns it (member_repo_key,
// where a request sends member_key).
type memberRow struct {
	Key      string `json:"member_repo_key"`
	Priority int    `json:"priority"`
}

// esc escapes s for use as one path segment.
func esc(s string) string { return url.PathEscape(s) }

// GetRepository returns the repository with this key.
func (c *Client) GetRepository(ctx context.Context, key string) (*Repository, error) {
	r := &Repository{}
	return r, c.do(ctx, http.MethodGet, "/repositories/"+esc(key), nil, r)
}

// CreateRepository creates a repository and returns it.
func (c *Client) CreateRepository(ctx context.Context, in CreateRepository) (*Repository, error) {
	r := &Repository{}
	return r, c.do(ctx, http.MethodPost, "/repositories", in, r)
}

// UpdateRepository changes the repository with this key (PATCH).
func (c *Client) UpdateRepository(ctx context.Context, key string, in UpdateRepository) error {
	return c.do(ctx, http.MethodPatch, "/repositories/"+esc(key), in, nil)
}

// DeleteRepository deletes the repository with this key.
func (c *Client) DeleteRepository(ctx context.Context, key string) error {
	return c.do(ctx, http.MethodDelete, "/repositories/"+esc(key), nil, nil)
}

// GetMembers returns the members of the virtual repository with this key,
// sorted by priority.
func (c *Client) GetMembers(ctx context.Context, key string) ([]Member, error) {
	var out struct {
		Members []memberRow `json:"members"`
	}
	if err := c.do(ctx, http.MethodGet, "/repositories/"+esc(key)+"/members", nil, &out); err != nil {
		return nil, err
	}
	m := make([]Member, 0, len(out.Members))
	for _, r := range out.Members {
		m = append(m, Member(r))
	}
	slices.SortStableFunc(m, func(a, b Member) int { return a.Priority - b.Priority })
	return m, nil
}

// SetMembers replaces the members of the virtual repository with this key.
func (c *Client) SetMembers(ctx context.Context, key string, m []Member) error {
	return c.do(ctx, http.MethodPut, "/repositories/"+esc(key)+"/members", map[string]any{"members": m}, nil)
}

// Group is a user group.
type Group struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	ExternalSource *string `json:"external_source"`
}

// FindGroup returns the group named name, or nil if there is none.
func (c *Client) FindGroup(ctx context.Context, name string) (*Group, error) {
	return findPaged(ctx, c, "/groups", name, func(g Group) string { return g.Name })
}

// findPaged returns the item of the paged list at path whose nameOf is name,
// or nil if there is none. The search can return items of other names too:
// only an exact name matches. The pages are read in turn until one matches
// or the last page is read.
func findPaged[T any](ctx context.Context, c *Client, path, name string, nameOf func(T) string) (*T, error) {
	for page := 1; ; page++ {
		var out struct {
			Items      []T        `json:"items"`
			Pagination pagination `json:"pagination"`
		}
		q := url.Values{"search": {name}, "page": {fmt.Sprint(page)}, "per_page": {"100"}}
		if err := c.do(ctx, http.MethodGet, path+"?"+q.Encode(), nil, &out); err != nil {
			return nil, err
		}
		for i := range out.Items {
			if nameOf(out.Items[i]) == name {
				return &out.Items[i], nil
			}
		}
		if page >= out.Pagination.TotalPages {
			return nil, nil
		}
	}
}

// User is a user account.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// FindUser returns the user named name, or nil if there is none.
func (c *Client) FindUser(ctx context.Context, name string) (*User, error) {
	return findPaged(ctx, c, "/users", name, func(u User) string { return u.Username })
}

// Permission grants a principal (PrincipalType group, user or
// service_account) actions on a target (TargetType repository, for this
// provider). ID is empty in a request body.
type Permission struct {
	ID            string   `json:"id,omitempty"`
	PrincipalType string   `json:"principal_type"`
	PrincipalID   string   `json:"principal_id"`
	TargetType    string   `json:"target_type"`
	TargetID      string   `json:"target_id"`
	Actions       []string `json:"actions"`
}

// GetPermission returns the permission with this ID.
func (c *Client) GetPermission(ctx context.Context, id string) (*Permission, error) {
	p := &Permission{}
	return p, c.do(ctx, http.MethodGet, "/permissions/"+esc(id), nil, p)
}

// FindPermission returns the permission of the principal on the target, or
// nil if there is none. The server filters by the four values, so the answer
// holds at most one permission (one per principal and target): one page is
// enough.
func (c *Client) FindPermission(ctx context.Context, principalType, principalID, targetType, targetID string) (*Permission, error) {
	var out struct {
		Items []Permission `json:"items"`
	}
	q := url.Values{"principal_type": {principalType}, "principal_id": {principalID},
		"target_type": {targetType}, "target_id": {targetID}, "per_page": {"100"}}
	if err := c.do(ctx, http.MethodGet, "/permissions?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	for i := range out.Items {
		p := out.Items[i]
		if p.PrincipalType == principalType && p.PrincipalID == principalID && p.TargetType == targetType && p.TargetID == targetID {
			return &p, nil
		}
	}
	return nil, nil
}

// CreatePermission creates a permission (in.ID is ignored) and returns it.
func (c *Client) CreatePermission(ctx context.Context, in Permission) (*Permission, error) {
	in.ID = ""
	p := &Permission{}
	return p, c.do(ctx, http.MethodPost, "/permissions", in, p)
}

// UpdatePermission replaces the permission with this ID by in (in.ID is
// ignored).
func (c *Client) UpdatePermission(ctx context.Context, id string, in Permission) error {
	in.ID = ""
	return c.do(ctx, http.MethodPut, "/permissions/"+esc(id), in, nil)
}

// DeletePermission deletes the permission with this ID.
func (c *Client) DeletePermission(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/permissions/"+esc(id), nil, nil)
}

// OIDCProvider is an OIDC login as Artifact Keeper returns it: never the
// client secret, only whether it has one (HasSecret).
type OIDCProvider struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	IssuerURL         string         `json:"issuer_url"`
	ClientID          string         `json:"client_id"`
	HasSecret         bool           `json:"has_secret"`
	Scopes            []string       `json:"scopes"`
	AttributeMapping  map[string]any `json:"attribute_mapping"`
	IsEnabled         bool           `json:"is_enabled"`
	AutoCreateUsers   bool           `json:"auto_create_users"`
	PKCEEnabled       bool           `json:"pkce_enabled"`
	MapGroupsToGroups bool           `json:"map_groups_to_groups"`
}

// OIDCProviderRequest is the body of an OIDC provider creation or update. An
// empty ClientSecret is left out.
type OIDCProviderRequest struct {
	Name                    string         `json:"name"`
	IssuerURL               string         `json:"issuer_url"`
	ClientID                string         `json:"client_id"`
	ClientSecret            string         `json:"client_secret,omitempty"`
	Scopes                  []string       `json:"scopes"`
	AttributeMapping        map[string]any `json:"attribute_mapping"`
	AttributeMappingReplace bool           `json:"attribute_mapping_replace,omitempty"`
	IsEnabled               bool           `json:"is_enabled"`
	AutoCreateUsers         bool           `json:"auto_create_users"`
	PKCEEnabled             bool           `json:"pkce_enabled"`
	MapGroupsToGroups       bool           `json:"map_groups_to_groups"`
	AdminGroup              *string        `json:"admin_group,omitempty"`
}

// GetOIDCProvider returns the OIDC provider with this ID.
func (c *Client) GetOIDCProvider(ctx context.Context, id string) (*OIDCProvider, error) {
	p := &OIDCProvider{}
	return p, c.do(ctx, http.MethodGet, "/admin/sso/oidc/"+esc(id), nil, p)
}

// FindOIDCProvider returns the OIDC provider named name, or nil if there is
// none. The list isn't paged.
func (c *Client) FindOIDCProvider(ctx context.Context, name string) (*OIDCProvider, error) {
	var out []OIDCProvider
	if err := c.do(ctx, http.MethodGet, "/admin/sso/oidc", nil, &out); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Name == name {
			return &out[i], nil
		}
	}
	return nil, nil
}

// CreateOIDCProvider creates an OIDC provider and returns it.
func (c *Client) CreateOIDCProvider(ctx context.Context, in OIDCProviderRequest) (*OIDCProvider, error) {
	p := &OIDCProvider{}
	return p, c.do(ctx, http.MethodPost, "/admin/sso/oidc", in, p)
}

// UpdateOIDCProvider replaces the settings of the OIDC provider with this ID.
// It replaces the attribute mapping as a whole (attribute_mapping_replace),
// so that keys the request no longer sets go away.
func (c *Client) UpdateOIDCProvider(ctx context.Context, id string, in OIDCProviderRequest) error {
	in.AttributeMappingReplace = true
	return c.do(ctx, http.MethodPut, "/admin/sso/oidc/"+esc(id), in, nil)
}

// DeleteOIDCProvider deletes the OIDC provider with this ID.
func (c *Client) DeleteOIDCProvider(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/admin/sso/oidc/"+esc(id), nil, nil)
}

// ServiceAccount is a service account; Username is svc-<name>.
type ServiceAccount struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"display_name"`
	IsActive    bool    `json:"is_active"`
}

// GetServiceAccount returns the service account with this ID.
func (c *Client) GetServiceAccount(ctx context.Context, id string) (*ServiceAccount, error) {
	s := &ServiceAccount{}
	return s, c.do(ctx, http.MethodGet, "/service-accounts/"+esc(id), nil, s)
}

// FindServiceAccount returns the service account whose user name is
// username (svc-<name>), or nil if there is none. The list isn't paged: it
// holds every account.
func (c *Client) FindServiceAccount(ctx context.Context, username string) (*ServiceAccount, error) {
	var out struct {
		Items []ServiceAccount `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/service-accounts", nil, &out); err != nil {
		return nil, err
	}
	for i := range out.Items {
		if out.Items[i].Username == username {
			return &out.Items[i], nil
		}
	}
	return nil, nil
}

// CreateServiceAccount makes svc-<name>: name is without the prefix, which
// Artifact Keeper adds.
func (c *Client) CreateServiceAccount(ctx context.Context, name, description string) (*ServiceAccount, error) {
	s := &ServiceAccount{}
	in := map[string]any{"name": name}
	if description != "" {
		in["description"] = description
	}
	return s, c.do(ctx, http.MethodPost, "/service-accounts", in, s)
}

// UpdateServiceAccount sets the display name and the active flag of the
// service account with this ID. A nil argument is sent as JSON null, which
// Artifact Keeper takes as "unchanged".
func (c *Client) UpdateServiceAccount(ctx context.Context, id string, displayName *string, active *bool) error {
	return c.do(ctx, http.MethodPatch, "/service-accounts/"+esc(id),
		map[string]any{"display_name": displayName, "is_active": active}, nil)
}

// DeleteServiceAccount deletes the service account with this ID.
func (c *Client) DeleteServiceAccount(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/service-accounts/"+esc(id), nil, nil)
}

// Token is an API token as the list returns it: never its value, only its
// first characters (Prefix).
type Token struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Prefix        string   `json:"token_prefix"`
	Scopes        []string `json:"scopes"`
	RepositoryIDs []string `json:"repository_ids"`
	ExpiresAt     *string  `json:"expires_at"`
	IsExpired     bool     `json:"is_expired"`
}

// CreateToken is the body of a token creation. No RepositoryIDs means the
// token isn't restricted to repositories; a nil ExpiresInDays leaves the
// expiry to the server.
type CreateToken struct {
	Name          string   `json:"name"`
	Scopes        []string `json:"scopes"`
	RepositoryIDs []string `json:"repository_ids,omitempty"`
	ExpiresInDays *int64   `json:"expires_in_days,omitempty"`
}

// CreatedToken carries the token, shown only once.
type CreatedToken struct {
	ID        string  `json:"id"`
	Token     string  `json:"token"`
	ExpiresAt *string `json:"expires_at"`
}

// ListTokens returns the tokens of the service account with this ID.
func (c *Client) ListTokens(ctx context.Context, serviceAccountID string) ([]Token, error) {
	var out struct {
		Items []Token `json:"items"`
	}
	return out.Items, c.do(ctx, http.MethodGet, "/service-accounts/"+esc(serviceAccountID)+"/tokens", nil, &out)
}

// CreateToken makes a token for the service account with this ID; the
// result holds its value, which can't be read again.
func (c *Client) CreateToken(ctx context.Context, serviceAccountID string, in CreateToken) (*CreatedToken, error) {
	t := &CreatedToken{}
	return t, c.do(ctx, http.MethodPost, "/service-accounts/"+esc(serviceAccountID)+"/tokens", in, t)
}

// RevokeToken revokes (deletes) a token of the service account.
func (c *Client) RevokeToken(ctx context.Context, serviceAccountID, tokenID string) error {
	return c.do(ctx, http.MethodDelete, "/service-accounts/"+esc(serviceAccountID)+"/tokens/"+esc(tokenID), nil, nil)
}
