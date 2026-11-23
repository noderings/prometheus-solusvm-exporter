package svmclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/noderings/prometheus-solusvm-exporter/internal/svmclient"
)

func TestGetAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/account" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing or wrong auth header: %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"id":1}}`))
	}))
	defer srv.Close()

	c := svmclient.New(srv.URL, "test-token", false)
	if err := c.GetAccount(context.Background()); err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
}

func TestListServers_Pagination(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/servers" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		page++
		switch page {
		case 1:
			resp := map[string]any{
				"data": []map[string]any{
					{"id": 1, "name": "NR-abc", "uuid": "uuid-1"},
				},
				"meta": map[string]any{"pagination": map[string]any{"current_page": 1, "total_pages": 2}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case 2:
			resp := map[string]any{
				"data": []map[string]any{
					{"id": 2, "name": "NR-def", "uuid": "uuid-2"},
				},
				"meta": map[string]any{"pagination": map[string]any{"current_page": 2, "total_pages": 2}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer srv.Close()

	c := svmclient.New(srv.URL, "tok", false)
	servers, err := c.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
}

func TestListComputeResources_NestedLocation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/compute_resources" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := `{"data":[{"id":1,"name":"cr1","status":"active","location":{"id":10},"settings":{"limits":{"vcpu":{"used":2,"total":8}}}}]}`
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()

	c := svmclient.New(srv.URL, "tok", false)
	crs, err := c.ListComputeResources(context.Background())
	if err != nil {
		t.Fatalf("ListComputeResources: %v", err)
	}
	if len(crs) != 1 || crs[0].ResolvedLocationID() != 10 {
		t.Fatalf("expected nested location.id=10, got %+v", crs)
	}
	if crs[0].ResolvedLimits().VCPU.Total != 8 {
		t.Fatalf("limits: %+v", crs[0].ResolvedLimits())
	}
}
