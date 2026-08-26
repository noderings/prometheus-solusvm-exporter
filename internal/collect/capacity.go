package collect

import (
	"context"
	"strconv"
	"time"

	"github.com/noderings/prometheus-solusvm-exporter/internal/metrics"
	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog/log"
)

type CapacityCollector struct {
	client SVMClient
}

func NewCapacityCollector(client SVMClient) *CapacityCollector {
	return &CapacityCollector{client: client}
}

const (
	mbToBytes = 1024 * 1024
	gbToBytes = 1024 * 1024 * 1024
)

// bytesFromLimit treats values already at byte scale as bytes, otherwise
// converts MiB (RAM) or GiB (disk). Lab dump may refine units.
func effectiveLimitTotal(total float64, unlimited bool, fallback float64) float64 {
	if unlimited || total <= 0 {
		return fallback
	}
	return total
}

func bytesFromLimit(v float64, disk bool) float64 {
	if v >= 1e9 {
		return v
	}
	if disk {
		return v * gbToBytes
	}
	return v * mbToBytes
}

// Collect fetches compute-resource limits (vcpu/ram/hdd used vs total).
func (cc *CapacityCollector) Collect(ctx context.Context) ([]*dto.MetricFamily, error) {
	var families []*dto.MetricFamily

	crs, err := cc.client.ListComputeResources(ctx)
	if err != nil {
		log.Error().Err(err).Msg("list compute resources failed")
		return nil, err
	}

	for _, cr := range crs {
		hid := strconv.Itoa(cr.ID)
		lim := cr.ResolvedLimits()
		// SVM "unlimited" (and total 0) means no CR cap. Emit a sellable max so
		// NodeRings preflight/capacity can compare against plan need.
		cpuMax := effectiveLimitTotal(lim.VCPU.Total, lim.VCPU.Unlimited, 256)
		ramMax := effectiveLimitTotal(lim.RAM.Total, lim.RAM.Unlimited, 0)
		if ramMax == 0 {
			ramMax = cr.Metrics.Memory.Total
		}
		if ramMax == 0 {
			ramMax = 32 * 1024 // MiB fallback when metrics.memory is also empty
		}
		diskMax := effectiveLimitTotal(lim.HDD.Total, lim.HDD.Unlimited, 10*1024) // GiB
		families = append(families,
			gaugeFamily(metrics.CPUAllocated, lim.VCPU.Used, hid),
			gaugeFamily(metrics.CPUMax, cpuMax, hid),
			gaugeFamily(metrics.MemoryAllocatedBytes, bytesFromLimit(lim.RAM.Used, false), hid),
			gaugeFamily(metrics.MemoryMaxBytes, bytesFromLimit(ramMax, false), hid),
			gaugeFamily(metrics.DiskAllocatedBytes, bytesFromLimit(lim.HDD.Used, true), hid),
			gaugeFamily(metrics.DiskMaxBytes, bytesFromLimit(diskMax, true), hid),
		)
	}

	metrics.ExporterLastSuccess.WithLabelValues("capacity").Set(float64(time.Now().Unix()))
	return families, nil
}
