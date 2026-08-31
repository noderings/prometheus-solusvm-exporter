package collect_test

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/noderings/prometheus-solusvm-exporter/internal/collect"
	"github.com/noderings/prometheus-solusvm-exporter/internal/svmclient"
)

func TestCapacityCollector_UnlimitedEmitsInf(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/compute_resources" {
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		resp := map[string]any{
			"data": []map[string]any{
				{
					"id":     1,
					"name":   "nuc2",
					"status": "active",
					"settings": map[string]any{
						"limits": map[string]any{
							"vcpu": map[string]any{"total": 0, "used": 0, "unlimited": true},
							"ram":  map[string]any{"total": 0, "used": 0, "unlimited": true},
							"hdd":  map[string]any{"total": 0, "used": 0, "unlimited": true},
						},
					},
					"metrics": map[string]any{
						"memory": map[string]any{"total": 33359044608.0},
					},
				},
			},
			"meta": map[string]any{"pagination": map[string]any{"current_page": 1, "total_pages": 1}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cc := collect.NewCapacityCollector(svmclient.New(srv.URL, "tok", false))
	families, err := cc.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	got := map[string]float64{}
	for _, f := range families {
		if len(f.Metric) == 0 || f.Metric[0].Gauge == nil {
			continue
		}
		got[f.GetName()] = f.Metric[0].Gauge.GetValue()
	}
	for _, name := range []string{"svm_cpu_max", "svm_memory_max_bytes", "svm_disk_max_bytes"} {
		v, ok := got[name]
		if !ok {
			t.Fatalf("missing %s", name)
		}
		if !math.IsInf(v, 1) {
			t.Fatalf("%s want +Inf, got %v", name, v)
		}
	}
	if got["svm_cpu_allocated"] != 0 || got["svm_memory_allocated_bytes"] != 0 {
		t.Fatalf("allocated should be 0, got %+v", got)
	}
}

func TestCapacityCollector_FiniteLimits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": []map[string]any{
				{
					"id": 2,
					"settings": map[string]any{
						"limits": map[string]any{
							"vcpu": map[string]any{"total": 8, "used": 2, "unlimited": false},
							"ram":  map[string]any{"total": 16384, "used": 4096, "unlimited": false},
							"hdd":  map[string]any{"total": 500, "used": 80, "unlimited": false},
						},
					},
				},
			},
			"meta": map[string]any{"pagination": map[string]any{"current_page": 1, "total_pages": 1}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cc := collect.NewCapacityCollector(svmclient.New(srv.URL, "tok", false))
	families, err := cc.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	got := map[string]float64{}
	for _, f := range families {
		if len(f.Metric) == 0 || f.Metric[0].Gauge == nil {
			continue
		}
		got[f.GetName()] = f.Metric[0].Gauge.GetValue()
	}
	if got["svm_cpu_max"] != 8 {
		t.Fatalf("cpu max: %v", got["svm_cpu_max"])
	}
	if got["svm_cpu_allocated"] != 2 {
		t.Fatalf("cpu allocated: %v", got["svm_cpu_allocated"])
	}
	wantMem := 16384.0 * 1024 * 1024
	if got["svm_memory_max_bytes"] != wantMem {
		t.Fatalf("mem max: %v want %v", got["svm_memory_max_bytes"], wantMem)
	}
	wantDisk := 500.0 * 1024 * 1024 * 1024
	if got["svm_disk_max_bytes"] != wantDisk {
		t.Fatalf("disk max: %v want %v", got["svm_disk_max_bytes"], wantDisk)
	}
}
