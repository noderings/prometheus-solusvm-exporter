package collect_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/noderings/prometheus-solusvm-exporter/internal/collect"
	dto "github.com/prometheus/client_model/go"
)

func familyValue(f *dto.MetricFamily) float64 {
	if len(f.Metric) == 0 {
		return 0
	}
	m := f.Metric[0]
	if m.Gauge != nil {
		return m.Gauge.GetValue()
	}
	if m.Counter != nil {
		return m.Counter.GetValue()
	}
	return 0
}

func TestMapUsage_EmptyItems(t *testing.T) {
	payloads := []string{
		`{"items":[]}`,
		`{"data":{"items":[]}}`,
		`{"data":[{"uuid":"u","items":[]}]}`,
		`{"data":[]}`,
		`{}`,
	}
	for _, raw := range payloads {
		families, err := collect.MapUsage("cpu", json.RawMessage(raw), "NR-a", "1", "u")
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if len(families) != 0 {
			t.Fatalf("empty items must emit no metrics, got %d for %s", len(families), raw)
		}
	}
}

func TestMapUsage_CPULoadAverage(t *testing.T) {
	raw := `{"items":[{"time":"2024-01-01T00:00:00Z","load_average":55.2},{"time":"2024-01-01T00:01:00Z","load_average":40}]}`
	families, err := collect.MapUsage("cpu", json.RawMessage(raw), "NR-test", "42", "abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 1 || families[0].GetName() != "svm_cpu_usage_ratio" {
		t.Fatalf("expected svm_cpu_usage_ratio, got %+v", families)
	}
	got := familyValue(families[0])
	if math.Abs(got-0.40) > 0.001 {
		t.Fatalf("latest load_average 40 → ratio 0.40, got %f", got)
	}

	rawRatio := `{"data":{"items":[{"load_average":0.42}]}}`
	families, err = collect.MapUsage("cpu", json.RawMessage(rawRatio), "n", "1", "u")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(familyValue(families[0])-0.42) > 0.001 {
		t.Fatalf("ratio load_average: got %f", familyValue(families[0]))
	}
}

func TestMapUsage_NetworkDiskGauges(t *testing.T) {
	raw := `{"items":[{"derivative":{"read_kb":10.5,"write_kb":20.1}}]}`
	net, err := collect.MapUsage("network", json.RawMessage(raw), "n", "1", "u")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, f := range net {
		got[f.GetName()] = familyValue(f)
		if f.GetType() != dto.MetricType_GAUGE {
			t.Fatalf("%s must be a gauge until lab dump", f.GetName())
		}
	}
	if got["svm_network_receive_bytes"] != 10.5 || got["svm_network_transmit_bytes"] != 20.1 {
		t.Fatalf("network: %+v", got)
	}

	disk, err := collect.MapUsage("disks", json.RawMessage(raw), "n", "1", "u")
	if err != nil {
		t.Fatal(err)
	}
	got = map[string]float64{}
	for _, f := range disk {
		got[f.GetName()] = familyValue(f)
	}
	if got["svm_disk_read_bytes"] != 10.5 || got["svm_disk_write_bytes"] != 20.1 {
		t.Fatalf("disks: %+v", got)
	}
}

func TestMapServerCPUFallback(t *testing.T) {
	raw := `{"data":{"id":1,"usage":{"cpu":12.5}}}`
	families := collect.MapServerCPUFallback(json.RawMessage(raw), "NR-a", "1", "u")
	if len(families) != 1 {
		t.Fatalf("expected fallback cpu, got %d", len(families))
	}
	if math.Abs(familyValue(families[0])-0.125) > 0.001 {
		t.Fatalf("got %f", familyValue(families[0]))
	}
}
