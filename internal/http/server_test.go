package http_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/noderings/prometheus-solusvm-exporter/internal/cache"
	svmhttp "github.com/noderings/prometheus-solusvm-exporter/internal/http"
	dto "github.com/prometheus/client_model/go"
)

func TestMetricsHandler_NoHTTPClient(t *testing.T) {
	snap := cache.NewSnapshot()

	name := "svm_up"
	help := "SolusVM 2 API reachable via GET /account"
	tp := dto.MetricType_GAUGE
	val := float64(1)
	snap.Update([]*dto.MetricFamily{
		{
			Name: &name,
			Help: &help,
			Type: &tp,
			Metric: []*dto.Metric{
				{Gauge: &dto.Gauge{Value: &val}},
			},
		},
	})

	mux := svmhttp.NewMux(snap)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	if !strings.Contains(text, "svm_up") {
		t.Error("expected svm_up in output")
	}
	if strings.Contains(text, "pve_") {
		t.Error("must not contain pve_* series")
	}
	if strings.Contains(text, "vf_") {
		t.Error("must not contain vf_* series")
	}
}

func TestHealthzHandler(t *testing.T) {
	snap := cache.NewSnapshot()
	mux := svmhttp.NewMux(snap)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
