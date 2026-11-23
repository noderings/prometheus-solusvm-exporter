package collect

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func anyInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	default:
		return 0
	}
}

func anyString(v any) string {
	s, _ := v.(string)
	return s
}

func anyBool(v any) bool {
	switch n := v.(type) {
	case bool:
		return n
	case float64:
		return n != 0
	case string:
		return strings.EqualFold(n, "true") || n == "1"
	default:
		return false
	}
}

func anyFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func gaugeFamily(desc *prometheus.Desc, value float64, labelValues ...string) *dto.MetricFamily {
	g := prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, labelValues...)
	m := &dto.Metric{}
	_ = g.Write(m)

	fqName := extractName(desc)
	help := extractHelp(desc)
	tp := dto.MetricType_GAUGE
	return &dto.MetricFamily{
		Name:   &fqName,
		Help:   &help,
		Type:   &tp,
		Metric: []*dto.Metric{m},
	}
}

func extractName(desc *prometheus.Desc) string {
	pd := desc.String()
	start := len("Desc{fqName: \"")
	end := start
	for end < len(pd) && pd[end] != '"' {
		end++
	}
	if end <= len(pd) {
		return pd[start:end]
	}
	return "unknown"
}

func extractHelp(desc *prometheus.Desc) string {
	pd := desc.String()
	const marker = "help: \""
	idx := 0
	for i := 0; i+len(marker) <= len(pd); i++ {
		if pd[i:i+len(marker)] == marker {
			idx = i + len(marker)
			break
		}
	}
	if idx == 0 {
		return ""
	}
	end := idx
	for end < len(pd) && pd[end] != '"' {
		end++
	}
	return pd[idx:end]
}
