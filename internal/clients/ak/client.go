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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client calls one Artifact Keeper server with an API token.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// New returns a client of the server at baseURL (https://host, no path).
// The token is an API token (Authorization: Bearer); TLS trust is the
// system's (SSL_CERT_DIR / SSL_CERT_FILE).
func New(baseURL, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), token: token, http: hc}
}

// APIError is an answer other than 2xx.
type APIError struct {
	Method, Path string
	Status       int
	Body         string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: %d %s", e.Method, e.Path, e.Status, e.Body)
}

// IsNotFound tells whether err is a 404 answer.
func IsNotFound(err error) bool {
	var e *APIError
	return asAPIError(err, &e) && e.Status == http.StatusNotFound
}

// IsConflict tells whether err is a 409 answer.
func IsConflict(err error) bool {
	var e *APIError
	return asAPIError(err, &e) && e.Status == http.StatusConflict
}

// asAPIError matches err itself, not what it wraps: this package returns
// APIErrors unwrapped, and callers test an error before wrapping it.
func asAPIError(err error, target **APIError) bool {
	e, ok := err.(*APIError) //nolint:errorlint // never wrapped in this package
	if ok {
		*target = e
	}
	return ok
}

// do sends in as JSON (when not nil) to /api/v1<path> and decodes the
// answer into out (when not nil). An answer other than 2xx is an APIError,
// with at most the first 300 characters of its body.
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
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		msg := strings.TrimSpace(string(b))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return &APIError{Method: method, Path: path, Status: res.StatusCode, Body: msg}
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}

// pagination is the paging information of a paged list.
type pagination struct {
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
}

// --- Repositories -----------------------------------------------------------

// Repository is GET /repositories/{key}.
type Repository struct {
	ID                   string  `json:"id"`
	Key                  string  `json:"key"`
	Name                 string  `json:"name"`
	Description          *string `json:"description"`
	Format               string  `json:"format"`
	RepoType             string  `json:"repo_type"`
	UpstreamURL          *string `json:"upstream_url"`
	AllowAnonymousAccess bool    `json:"allow_anonymous_access"`
	QuotaBytes           *int64  `json:"quota_bytes"`
	StorageUsedBytes     int64   `json:"storage_used_bytes"`
	CurationDefaultMode  string  `json:"curation_default_action"`
}

// CreateRepository is POST /repositories.
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

// UpdateRepository is PATCH /repositories/{key}: unset fields stay.
// CurationDefaultAction is required by the API: send the current one.
type UpdateRepository struct {
	Name                  *string `json:"name,omitempty"`
	Description           *string `json:"description,omitempty"`
	AllowAnonymousAccess  *bool   `json:"allow_anonymous_access,omitempty"`
	QuotaBytes            *int64  `json:"quota_bytes,omitempty"`
	CurationDefaultAction string  `json:"curation_default_action"`
}

// Member is a member of a virtual repository, as PUT .../members takes it.
type Member struct {
	Key      string `json:"member_key"`
	Priority int    `json:"priority"`
}

// memberRow is a member as GET .../members returns it: the same, under
// another key name (member_repo_key).
type memberRow struct {
	Key      string `json:"member_repo_key"`
	Priority int    `json:"priority"`
}

func esc(s string) string { return url.PathEscape(s) }

// GetRepository returns the repository with this key.
func (c *Client) GetRepository(ctx context.Context, key string) (*Repository, error) {
	r := &Repository{}
	return r, c.do(ctx, http.MethodGet, "/repositories/"+esc(key), nil, r)
}

// CreateRepository makes a repository.
func (c *Client) CreateRepository(ctx context.Context, in CreateRepository) (*Repository, error) {
	r := &Repository{}
	return r, c.do(ctx, http.MethodPost, "/repositories", in, r)
}

// UpdateRepository changes a repository's settings.
func (c *Client) UpdateRepository(ctx context.Context, key string, in UpdateRepository) error {
	return c.do(ctx, http.MethodPatch, "/repositories/"+esc(key), in, nil)
}

// DeleteRepository deletes a repository and its packages.
func (c *Client) DeleteRepository(ctx context.Context, key string) error {
	return c.do(ctx, http.MethodDelete, "/repositories/"+esc(key), nil, nil)
}

// GetMembers returns a virtual repository's members, by priority.
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
	sortMembers(m)
	return m, nil
}

// SetMembers replaces a virtual repository's members.
func (c *Client) SetMembers(ctx context.Context, key string, m []Member) error {
	return c.do(ctx, http.MethodPut, "/repositories/"+esc(key)+"/members", map[string]any{"members": m}, nil)
}

func sortMembers(m []Member) {
	for i := 1; i < len(m); i++ {
		for j := i; j > 0 && m[j].Priority < m[j-1].Priority; j-- {
			m[j], m[j-1] = m[j-1], m[j]
		}
	}
}

// --- Groups and users -------------------------------------------------------

// Group is an item of GET /groups.
type Group struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	ExternalSource *string `json:"external_source"`
}

// FindGroup returns the group with exactly this name, or nil.
func (c *Client) FindGroup(ctx context.Context, name string) (*Group, error) {
	for page := 1; ; page++ {
		var out struct {
			Items      []Group    `json:"items"`
			Pagination pagination `json:"pagination"`
		}
		q := url.Values{"search": {name}, "page": {fmt.Sprint(page)}, "per_page": {"100"}}
		if err := c.do(ctx, http.MethodGet, "/groups?"+q.Encode(), nil, &out); err != nil {
			return nil, err
		}
		for i := range out.Items {
			if out.Items[i].Name == name {
				return &out.Items[i], nil
			}
		}
		if page >= out.Pagination.TotalPages {
			return nil, nil
		}
	}
}

// User is an item of GET /users.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// FindUser returns the user (or service account) with exactly this name,
// or nil.
func (c *Client) FindUser(ctx context.Context, name string) (*User, error) {
	for page := 1; ; page++ {
		var out struct {
			Items      []User     `json:"items"`
			Pagination pagination `json:"pagination"`
		}
		q := url.Values{"search": {name}, "page": {fmt.Sprint(page)}, "per_page": {"100"}}
		if err := c.do(ctx, http.MethodGet, "/users?"+q.Encode(), nil, &out); err != nil {
			return nil, err
		}
		for i := range out.Items {
			if out.Items[i].Username == name {
				return &out.Items[i], nil
			}
		}
		if page >= out.Pagination.TotalPages {
			return nil, nil
		}
	}
}

// --- Permissions ------------------------------------------------------------

// Permission is GET /permissions/{id}; also the body of POST and PUT
// (without ID).
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

// FindPermission returns the permission of this principal on this target,
// or nil. The server filters by the four values, so the answer holds at
// most one permission (one per principal and target): one page is enough.
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

// CreatePermission grants a permission.
func (c *Client) CreatePermission(ctx context.Context, in Permission) (*Permission, error) {
	in.ID = ""
	p := &Permission{}
	return p, c.do(ctx, http.MethodPost, "/permissions", in, p)
}

// UpdatePermission replaces a permission.
func (c *Client) UpdatePermission(ctx context.Context, id string, in Permission) error {
	in.ID = ""
	return c.do(ctx, http.MethodPut, "/permissions/"+esc(id), in, nil)
}

// DeletePermission revokes a permission.
func (c *Client) DeletePermission(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/permissions/"+esc(id), nil, nil)
}

// --- OIDC providers ---------------------------------------------------------

// OIDCProvider is GET /admin/sso/oidc/{id}.
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

// OIDCProviderRequest is the body of POST and PUT /admin/sso/oidc: the
// whole desired configuration, client secret included.
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

// GetOIDCProvider returns the provider with this ID.
func (c *Client) GetOIDCProvider(ctx context.Context, id string) (*OIDCProvider, error) {
	p := &OIDCProvider{}
	return p, c.do(ctx, http.MethodGet, "/admin/sso/oidc/"+esc(id), nil, p)
}

// FindOIDCProvider returns the provider with this name, or nil.
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

// CreateOIDCProvider adds an OIDC login.
func (c *Client) CreateOIDCProvider(ctx context.Context, in OIDCProviderRequest) (*OIDCProvider, error) {
	p := &OIDCProvider{}
	return p, c.do(ctx, http.MethodPost, "/admin/sso/oidc", in, p)
}

// UpdateOIDCProvider changes an OIDC login. It replaces the attribute
// mapping as a whole (attribute_mapping_replace), so that keys the request
// no longer sets go away.
func (c *Client) UpdateOIDCProvider(ctx context.Context, id string, in OIDCProviderRequest) error {
	in.AttributeMappingReplace = true
	return c.do(ctx, http.MethodPut, "/admin/sso/oidc/"+esc(id), in, nil)
}

// DeleteOIDCProvider removes an OIDC login.
func (c *Client) DeleteOIDCProvider(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/admin/sso/oidc/"+esc(id), nil, nil)
}

// --- Service accounts and tokens --------------------------------------------

// ServiceAccount is GET /service-accounts/{id}.
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

// FindServiceAccount returns the service account with this user name
// (svc-<name>), or nil. The list isn't paged: it holds every account.
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

// UpdateServiceAccount changes display name and activity. A nil one is sent
// as JSON null, which Artifact Keeper takes as "unchanged".
func (c *Client) UpdateServiceAccount(ctx context.Context, id string, displayName *string, active *bool) error {
	return c.do(ctx, http.MethodPatch, "/service-accounts/"+esc(id),
		map[string]any{"display_name": displayName, "is_active": active}, nil)
}

// DeleteServiceAccount deletes a service account (and its tokens).
func (c *Client) DeleteServiceAccount(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/service-accounts/"+esc(id), nil, nil)
}

// Token is an item of GET /service-accounts/{id}/tokens.
type Token struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Prefix        string   `json:"token_prefix"`
	Scopes        []string `json:"scopes"`
	RepositoryIDs []string `json:"repository_ids"`
	ExpiresAt     *string  `json:"expires_at"`
	IsExpired     bool     `json:"is_expired"`
}

// CreateToken is POST /service-accounts/{id}/tokens.
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

// ListTokens returns a service account's tokens.
func (c *Client) ListTokens(ctx context.Context, serviceAccountID string) ([]Token, error) {
	var out struct {
		Items []Token `json:"items"`
	}
	return out.Items, c.do(ctx, http.MethodGet, "/service-accounts/"+esc(serviceAccountID)+"/tokens", nil, &out)
}

// CreateToken makes a token of a service account.
func (c *Client) CreateToken(ctx context.Context, serviceAccountID string, in CreateToken) (*CreatedToken, error) {
	t := &CreatedToken{}
	return t, c.do(ctx, http.MethodPost, "/service-accounts/"+esc(serviceAccountID)+"/tokens", in, t)
}

// RevokeToken revokes a token.
func (c *Client) RevokeToken(ctx context.Context, serviceAccountID, tokenID string) error {
	return c.do(ctx, http.MethodDelete, "/service-accounts/"+esc(serviceAccountID)+"/tokens/"+esc(tokenID), nil, nil)
}
