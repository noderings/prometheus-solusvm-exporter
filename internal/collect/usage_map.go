package collect

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/noderings/prometheus-solusvm-exporter/internal/metrics"
	dto "github.com/prometheus/client_model/go"
)

func mapFloat(m map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if f, ok := anyFloat(v); ok {
				return f, true
			}
		}
	}
	return 0, false
}

func asObject(raw json.RawMessage) map[string]any {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

func unwrapData(raw json.RawMessage) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return raw
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && len(bytes.TrimSpace(envelope.Data)) > 0 {
		return envelope.Data
	}
	return raw
}

func extractItems(raw json.RawMessage) []map[string]any {
	raw = unwrapData(raw)
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}

	tryItems := func(m map[string]any) []map[string]any {
		rawItems, ok := m["items"]
		if !ok || rawItems == nil {
			return nil
		}
		arr, ok := rawItems.([]any)
		if !ok {
			return nil
		}
		out := make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			im, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, im)
		}
		return out
	}

	if raw[0] == '{' {
		m := asObject(raw)
		if m == nil {
			return nil
		}
		if items := tryItems(m); items != nil {
			return items
		}
		return nil
	}

	if raw[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil
		}
		if len(arr) == 0 {
			return nil
		}
		if items := tryItems(asObject(arr[0])); items != nil {
			return items
		}
	}
	return nil
}

func latestItem(items []map[string]any) map[string]any {
	if len(items) == 0 {
		return nil
	}
	best := items[len(items)-1]
	bestKey := itemTimeKey(best)
	for _, it := range items {
		k := itemTimeKey(it)
		if k >= bestKey {
			best = it
			bestKey = k
		}
	}
	return best
}

func itemTimeKey(m map[string]any) string {
	if s := anyString(m["time"]); s != "" {
		return s
	}
	if s := anyString(m["date"]); s != "" {
		return s
	}
	if f, ok := anyFloat(m["time"]); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return ""
}

func cpuRatioFromLoad(v float64) float64 {
	if v > 1 {
		return v / 100.0
	}
	return v
}

func derivativePair(m map[string]any) (read, write float64, ok bool) {
	der, _ := m["derivative"].(map[string]any)
	if der == nil {
		return 0, 0, false
	}
	r, rok := mapFloat(der, "read_kb", "read")
	w, wok := mapFloat(der, "write_kb", "write")
	return r, w, rok || wok
}

// MapUsage maps a WHMCS-shaped SolusVM usage payload to Prometheus families.
// Empty items (no qemu-guest-agent) yield no metrics and no error.
func MapUsage(kind string, raw json.RawMessage, name, serverID, uuid string) ([]*dto.MetricFamily, error) {
	items := extractItems(raw)
	if len(items) == 0 {
		return nil, nil
	}
	latest := latestItem(items)
	if latest == nil {
		return nil, nil
	}
	labels := []string{name, serverID, uuid}
	kind = strings.ToLower(strings.TrimSpace(kind))

	switch kind {
	case "cpu":
		v, ok := mapFloat(latest, "load_average", "cpu")
		if !ok {
			return nil, nil
		}
		return []*dto.MetricFamily{gaugeFamily(metrics.CPUUsageRatio, cpuRatioFromLoad(v), labels...)}, nil
	case "memory":
		v, ok := mapFloat(latest, "memory")
		if !ok {
			return nil, nil
		}
		return []*dto.MetricFamily{gaugeFamily(metrics.MemoryUsageBytes, v, labels...)}, nil
	case "network":
		r, w, ok := derivativePair(latest)
		if !ok {
			return nil, nil
		}
		return []*dto.MetricFamily{
			gaugeFamily(metrics.NetworkReceiveBytes, r, labels...),
			gaugeFamily(metrics.NetworkTransmitBytes, w, labels...),
		}, nil
	case "disks", "disk":
		r, w, ok := derivativePair(latest)
		if !ok {
			return nil, nil
		}
		return []*dto.MetricFamily{
			gaugeFamily(metrics.DiskReadBytes, r, labels...),
			gaugeFamily(metrics.DiskWriteBytes, w, labels...),
		}, nil
	default:
		return nil, nil
	}
}

// MapServerCPUFallback reads GET /servers/{id} usage.cpu when usage/cpu items are empty.
func MapServerCPUFallback(raw json.RawMessage, name, serverID, uuid string) []*dto.MetricFamily {
	raw = unwrapData(raw)
	m := asObject(raw)
	if m == nil {
		return nil
	}
	usage, _ := m["usage"].(map[string]any)
	if usage == nil {
		return nil
	}
	v, ok := mapFloat(usage, "cpu")
	if !ok {
		return nil
	}
	return []*dto.MetricFamily{gaugeFamily(metrics.CPUUsageRatio, cpuRatioFromLoad(v), name, serverID, uuid)}
}
