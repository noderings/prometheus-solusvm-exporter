package collect

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/noderings/prometheus-solusvm-exporter/internal/metrics"
	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog/log"
)

type GuestCollector struct {
	client      SVMClient
	concurrency int
	timeout     time.Duration
	namePrefix  string
}

func NewGuestCollector(client SVMClient, concurrency int, timeout time.Duration, namePrefix string) *GuestCollector {
	return &GuestCollector{
		client:      client,
		concurrency: concurrency,
		timeout:     timeout,
		namePrefix:  namePrefix,
	}
}

var usageKinds = []string{"cpu", "memory", "network", "disks"}

// Collect fetches usage for running NR- servers concurrently.
func (gc *GuestCollector) Collect(ctx context.Context, servers []ServerSummary) []*dto.MetricFamily {
	start := time.Now()
	defer func() {
		metrics.GuestScrapeSeconds.Observe(time.Since(start).Seconds())
	}()

	filtered := FilterServers(servers, gc.namePrefix)
	if len(filtered) == 0 {
		return nil
	}

	type result struct {
		families []*dto.MetricFamily
	}

	results := make([]result, len(filtered))
	sem := make(chan struct{}, gc.concurrency)
	var wg sync.WaitGroup

	for i, s := range filtered {
		wg.Add(1)
		go func(idx int, srv ServerSummary) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			callCtx, cancel := context.WithTimeout(ctx, gc.timeout)
			defer cancel()

			sid := strconv.Itoa(srv.ID)
			var families []*dto.MetricFamily
			cpuEmitted := false

			for _, kind := range usageKinds {
				raw, err := gc.client.GetUsage(callCtx, kind, srv.UUID)
				if err != nil {
					log.Warn().Err(err).Int("server_id", srv.ID).Str("kind", kind).Msg("usage fetch failed")
					metrics.UsageErrorsTotal.WithLabelValues("fetch").Inc()
					continue
				}
				mapped, err := MapUsage(kind, raw, srv.Name, sid, srv.UUID)
				if err != nil {
					log.Warn().Err(err).Int("server_id", srv.ID).Str("kind", kind).Msg("mapUsage failed")
					metrics.UsageErrorsTotal.WithLabelValues("parse").Inc()
					continue
				}
				if kind == "cpu" && len(mapped) > 0 {
					cpuEmitted = true
				}
				families = append(families, mapped...)
			}

			if !cpuEmitted {
				raw, err := gc.client.GetServer(callCtx, srv.ID)
				if err != nil {
					log.Warn().Err(err).Int("server_id", srv.ID).Msg("server fallback failed")
					metrics.UsageErrorsTotal.WithLabelValues("fallback").Inc()
				} else {
					families = append(families, MapServerCPUFallback(raw, srv.Name, sid, srv.UUID)...)
				}
			}

			results[idx] = result{families: families}
		}(i, s)
	}

	wg.Wait()

	var allFamilies []*dto.MetricFamily
	for _, r := range results {
		allFamilies = append(allFamilies, r.families...)
	}

	metrics.ExporterLastSuccess.WithLabelValues("guest").Set(float64(time.Now().Unix()))
	return allFamilies
}
