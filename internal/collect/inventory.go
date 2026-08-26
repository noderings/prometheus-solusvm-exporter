package collect

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/noderings/prometheus-solusvm-exporter/internal/metrics"
	"github.com/noderings/prometheus-solusvm-exporter/internal/svmclient"
	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog/log"
)

// SVMClient is the subset of svmclient.Client used by collectors.
type SVMClient interface {
	GetAccount(ctx context.Context) error
	ListServers(ctx context.Context) ([]json.RawMessage, error)
	ListComputeResources(ctx context.Context) ([]svmclient.ComputeResource, error)
	GetUsage(ctx context.Context, kind, uuid string) (json.RawMessage, error)
	GetServer(ctx context.Context, serverID int) (json.RawMessage, error)
}

// ServerSummary is the subset of fields used from a server listing.
type ServerSummary struct {
	ID                int    `json:"id"`
	UUID              string `json:"uuid"`
	Name              string `json:"name"`
	ComputeResourceID int    `json:"compute_resource_id"`
	LocationID        int    `json:"location_id"`
	Status            string `json:"status"`
	IsSuspended       bool   `json:"is_suspended"`
}

func parseServerSummary(raw json.RawMessage) (ServerSummary, error) {
	var extra map[string]any
	if err := json.Unmarshal(raw, &extra); err != nil {
		return ServerSummary{}, err
	}
	s := ServerSummary{
		ID:                anyInt(extra["id"]),
		UUID:              anyString(extra["uuid"]),
		Name:              anyString(extra["name"]),
		ComputeResourceID: anyInt(extra["compute_resource_id"]),
		LocationID:        anyInt(extra["location_id"]),
		Status:            anyString(extra["status"]),
		IsSuspended:       anyBool(extra["is_suspended"]),
	}
	if s.ComputeResourceID == 0 {
		s.ComputeResourceID = anyInt(extra["computeResourceId"])
	}
	if s.ComputeResourceID == 0 {
		if cr, ok := extra["compute_resource"].(map[string]any); ok {
			s.ComputeResourceID = anyInt(cr["id"])
		}
	}
	if s.LocationID == 0 {
		if loc, ok := extra["location"].(map[string]any); ok {
			s.LocationID = anyInt(loc["id"])
		}
	}
	if s.Status == "" {
		s.Status = anyString(extra["state"])
	}
	if !s.IsSuspended {
		s.IsSuspended = anyBool(extra["suspended"])
	}
	return s, nil
}

type InventoryCollector struct {
	client     SVMClient
	namePrefix string
}

func NewInventoryCollector(client SVMClient, namePrefix string) *InventoryCollector {
	return &InventoryCollector{client: client, namePrefix: namePrefix}
}

// Collect runs GET /account, GET /compute_resources, and GET /servers (paginated).
// svm_server_info is emitted only for NodeRings names (NR- prefix).
func (ic *InventoryCollector) Collect(ctx context.Context) ([]*dto.MetricFamily, []ServerSummary, error) {
	var families []*dto.MetricFamily

	err := ic.client.GetAccount(ctx)
	if err != nil {
		log.Error().Err(err).Msg("SolusVM account check failed")
		families = append(families, gaugeFamily(metrics.Up, 0))
		return families, nil, err
	}
	families = append(families, gaugeFamily(metrics.Up, 1))

	crs, err := ic.client.ListComputeResources(ctx)
	if err != nil {
		log.Error().Err(err).Msg("list compute resources failed")
		return families, nil, err
	}
	for _, cr := range crs {
		up := float64(0)
		if cr.IsUp() {
			up = 1
		}
		families = append(families, gaugeFamily(metrics.ComputeResourceUp, up,
			strconv.Itoa(cr.ID),
			strconv.Itoa(cr.ResolvedLocationID()),
			cr.Name,
		))
	}

	rawServers, err := ic.client.ListServers(ctx)
	if err != nil {
		log.Error().Err(err).Msg("list servers failed")
		return families, nil, err
	}

	var servers []ServerSummary
	for _, raw := range rawServers {
		s, err := parseServerSummary(raw)
		if err != nil {
			log.Warn().Err(err).Msg("skip unparseable server")
			continue
		}
		servers = append(servers, s)
	}

	for _, s := range servers {
		if !HasNamePrefix(s.Name, ic.namePrefix) {
			continue
		}
		families = append(families, gaugeFamily(metrics.ServerInfo, 1,
			s.Name,
			strconv.Itoa(s.ID),
			s.UUID,
			strconv.Itoa(s.ComputeResourceID),
		))
		up := float64(0)
		if s.IsRunning() {
			up = 1
		}
		families = append(families, gaugeFamily(metrics.GuestUp, up,
			s.Name,
			strconv.Itoa(s.ID),
			s.UUID,
		))
	}

	metrics.ExporterLastSuccess.WithLabelValues("inventory").Set(float64(time.Now().Unix()))

	return families, servers, nil
}

// HasNamePrefix reports whether name starts with prefix (case-insensitive).
// Empty prefix matches everything.
func HasNamePrefix(name, prefix string) bool {
	if prefix == "" {
		return true
	}
	return len(name) >= len(prefix) && strings.EqualFold(name[:len(prefix)], prefix)
}

// FilterServers returns NodeRings servers that look running. Used before
// expensive GET /usage/{kind}/{uuid}.
func FilterServers(servers []ServerSummary, prefix string) []ServerSummary {
	var out []ServerSummary
	for _, s := range servers {
		if !HasNamePrefix(s.Name, prefix) {
			continue
		}
		if !s.IsRunning() {
			continue
		}
		out = append(out, s)
	}
	return out
}

// IsRunning is best-effort from the listing (started, not suspended).
func (s ServerSummary) IsRunning() bool {
	if s.IsSuspended {
		return false
	}
	st := strings.ToLower(strings.TrimSpace(s.Status))
	switch st {
	case "started", "running", "active":
		return true
	default:
		return false
	}
}
