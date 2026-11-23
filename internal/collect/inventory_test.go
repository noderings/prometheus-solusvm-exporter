package collect_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/noderings/prometheus-solusvm-exporter/internal/collect"
	"github.com/noderings/prometheus-solusvm-exporter/internal/svmclient"
)

func TestFilterServers_PrefixAndStatus(t *testing.T) {
	servers := []collect.ServerSummary{
		{ID: 1, Name: "NR-abc", Status: "started"},
		{ID: 2, Name: "NR-def", Status: "stopped"},
		{ID: 3, Name: "other-vm", Status: "started"},
		{ID: 4, Name: "NR-ghi", Status: "running"},
		{ID: 5, Name: "nr-lowercase", Status: "started"},
		{ID: 6, Name: "NR-jkl", Status: "started", IsSuspended: true},
	}

	filtered := collect.FilterServers(servers, "NR-")

	if len(filtered) != 3 {
		t.Fatalf("expected 3 servers, got %d: %+v", len(filtered), filtered)
	}

	names := map[string]bool{}
	for _, s := range filtered {
		names[s.Name] = true
	}

	if !names["NR-abc"] || !names["NR-ghi"] || !names["nr-lowercase"] {
		t.Fatalf("unexpected names: %v", names)
	}
	if names["NR-def"] || names["other-vm"] || names["NR-jkl"] {
		t.Fatalf("unexpected extra names: %v", names)
	}
}

func TestHasNamePrefix(t *testing.T) {
	if !collect.HasNamePrefix("NR-abc", "NR-") {
		t.Fatal("NR-abc")
	}
	if !collect.HasNamePrefix("nr-abc", "NR-") {
		t.Fatal("nr-abc should match NR-")
	}
	if collect.HasNamePrefix("other", "NR-") {
		t.Fatal("other should not match")
	}
	if !collect.HasNamePrefix("anything", "") {
		t.Fatal("empty prefix matches all")
	}
}

func TestInventoryCollector_EmitsOnlyNRServerInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/account":
			_, _ = w.Write([]byte(`{"data":{"id":1,"email":"admin@example.com"}}`))
		case "/api/v1/compute_resources":
			resp := map[string]any{
				"data": []map[string]any{
					{"id": 11, "name": "cr1", "status": "active", "is_locked": false, "location": map[string]any{"id": 2}},
				},
				"meta": map[string]any{"pagination": map[string]any{"current_page": 1, "total_pages": 1}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "/api/v1/servers":
			resp := map[string]any{
				"data": []map[string]any{
					{"id": 1, "name": "NR-abc", "uuid": "u1", "compute_resource_id": 11, "status": "started"},
					{"id": 2, "name": "customer-vm", "uuid": "u2", "compute_resource_id": 11, "status": "started"},
					{"id": 3, "name": "nr-lower", "uuid": "u3", "compute_resource_id": 12, "status": "started"},
				},
				"meta": map[string]any{"pagination": map[string]any{"current_page": 1, "total_pages": 1}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ic := collect.NewInventoryCollector(svmclient.New(srv.URL, "tok", false), "NR-")
	families, servers, err := ic.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(servers) != 3 {
		t.Fatalf("full list should include non-NR servers, got %d", len(servers))
	}

	var infoNames []string
	var upNames []string
	guestUp := 0
	for _, f := range families {
		switch f.GetName() {
		case "svm_server_info":
			for _, l := range f.Metric[0].Label {
				if l.GetName() == "name" {
					infoNames = append(infoNames, l.GetValue())
				}
			}
		case "svm_compute_resource_up":
			for _, l := range f.Metric[0].Label {
				if l.GetName() == "name" {
					upNames = append(upNames, l.GetValue())
				}
			}
		case "svm_up":
			if f.Metric[0].Gauge.GetValue() != 1 {
				t.Fatalf("svm_up want 1")
			}
		case "svm_guest_up":
			guestUp += 1
		}
	}
	if len(infoNames) != 2 {
		t.Fatalf("svm_server_info should be NR-only, got %v", infoNames)
	}
	got := map[string]bool{}
	for _, n := range infoNames {
		got[n] = true
	}
	if !got["NR-abc"] || !got["nr-lower"] || got["customer-vm"] {
		t.Fatalf("unexpected svm_server_info names: %v", infoNames)
	}
	if len(upNames) != 1 || upNames[0] != "cr1" {
		t.Fatalf("svm_compute_resource_up names: %v", upNames)
	}
	if guestUp != 2 {
		t.Fatalf("svm_guest_up series want 2 NR guests, got %d", guestUp)
	}
}
