// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package ak

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestErrorsAndAuth checks the bearer token, a 404 as not found, a 401 as
// another error, and members sorted by priority.
func TestErrorsAndAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/repositories/missing":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"NOT_FOUND"}`))
		case "/api/v1/repositories/maven":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "1", "key": "maven", "format": "maven", "repo_type": "local"})
		case "/api/v1/repositories/virt/members":
			_ = json.NewEncoder(w).Encode(map[string]any{"members": []map[string]any{
				{"member_repo_key": "b", "priority": 2}, {"member_repo_key": "a", "priority": 1}}})
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", srv.Client())
	ctx := context.Background()

	if _, err := c.GetRepository(ctx, "missing"); !IsNotFound(err) {
		t.Fatalf("want not found, got %v", err)
	}
	r, err := c.GetRepository(ctx, "maven")
	if err != nil || r.Key != "maven" || r.RepoType != "local" {
		t.Fatalf("got %+v, %v", r, err)
	}
	m, err := c.GetMembers(ctx, "virt")
	if err != nil || len(m) != 2 || m[0].Key != "a" || m[1].Key != "b" {
		t.Fatalf("members not sorted by priority: %+v, %v", m, err)
	}
	if _, err := New(srv.URL, "wrong", srv.Client()).GetRepository(ctx, "maven"); err == nil || IsNotFound(err) {
		t.Fatalf("a 401 must be an error other than not found: %v", err)
	}
}

// TestFindGroupPages checks that FindGroup skips a partial match on page 1
// and finds the exact name on page 2.
func TestFindGroupPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		items := []map[string]any{{"id": "x", "name": "team-platform-old"}}
		if page == "2" {
			items = []map[string]any{{"id": "g", "name": "team-platform"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "pagination": map[string]any{"total_pages": 2}})
	}))
	defer srv.Close()
	g, err := New(srv.URL, "t", srv.Client()).FindGroup(context.Background(), "team-platform")
	if err != nil || g == nil || g.ID != "g" {
		t.Fatalf("exact name on page 2: %+v, %v", g, err)
	}
}
