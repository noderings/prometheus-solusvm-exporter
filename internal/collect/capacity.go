package collect

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/noderings/prometheus-solusvm-exporter/internal/metrics"
	"github.com/noderings/prometheus-solusvm-exporter/internal/svmclient"
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

// unlimitedMax is Prometheus +Inf: SolusVM settings.limits.*.unlimited (or
// total 0) means the compute resource has no cap. NodeRings must show
// Unlimited and skip sell/physical-fit against a fake number.
func unlimitedMax() float64 {
	return math.Inf(1)
}

func limitIsUnlimited(pool svmclient.LimitPool) bool {
	return pool.Unlimited || pool.Total <= 0
}

// cpuMaxFromLimit returns vCPU total, or +Inf when SolusVM has no vCPU cap.
func cpuMaxFromLimit(vcpu svmclient.LimitPool) float64 {
	if limitIsUnlimited(vcpu) {
		return unlimitedMax()
	}
	return vcpu.Total
}

// bytesFromLimit treats values already at byte scale as bytes, otherwise
// converts MiB (RAM) or GiB (disk).
func bytesFromLimit(v float64, disk bool) float64 {
	if v >= 1e9 {
		return v
	}
	if disk {
		return v * gbToBytes
	}
	return v * mbToBytes
}

func bytesMaxFromLimit(pool svmclient.LimitPool, disk bool) float64 {
	if limitIsUnlimited(pool) {
		return unlimitedMax()
	}
	return bytesFromLimit(pool.Total, disk)
}

func bytesAllocatedFromLimit(pool svmclient.LimitPool, disk bool) float64 {
	if pool.Used <= 0 {
		return 0
	}
	return bytesFromLimit(pool.Used, disk)
}

// Collect fetches compute-resource limits (vcpu/ram/hdd used vs total).
// Unlimited SolusVM caps are emitted as +Inf, not a invented 256 / 10 TiB / host RAM.
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
		families = append(families,
			gaugeFamily(metrics.CPUAllocated, lim.VCPU.Used, hid),
			gaugeFamily(metrics.CPUMax, cpuMaxFromLimit(lim.VCPU), hid),
			gaugeFamily(metrics.MemoryAllocatedBytes, bytesAllocatedFromLimit(lim.RAM, false), hid),
			gaugeFamily(metrics.MemoryMaxBytes, bytesMaxFromLimit(lim.RAM, false), hid),
			gaugeFamily(metrics.DiskAllocatedBytes, bytesAllocatedFromLimit(lim.HDD, true), hid),
			gaugeFamily(metrics.DiskMaxBytes, bytesMaxFromLimit(lim.HDD, true), hid),
		)
	}

	metrics.ExporterLastSuccess.WithLabelValues("capacity").Set(float64(time.Now().Unix()))
	return families, nil
}
