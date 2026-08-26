package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/noderings/prometheus-solusvm-exporter/internal/cache"
	"github.com/noderings/prometheus-solusvm-exporter/internal/collect"
	svmhttp "github.com/noderings/prometheus-solusvm-exporter/internal/http"
	"github.com/noderings/prometheus-solusvm-exporter/internal/svmclient"
	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	var (
		listen           = flag.String("listen", envOr("LISTEN_ADDR", ":9179"), "listen address")
		svmURL           = flag.String("solusvm-url", os.Getenv("SOLUSVM_URL"), "SolusVM 2 management URL (no /api/v1)")
		svmTokenFile     = flag.String("solusvm-token-file", "", "path to bearer token file")
		refreshInventory = flag.Duration("refresh-inventory", 30*time.Second, "inventory refresh interval")
		refreshCapacity  = flag.Duration("refresh-capacity", 60*time.Second, "capacity refresh interval")
		refreshGuest     = flag.Duration("refresh-guest", 45*time.Second, "guest metrics refresh interval")
		guestConcurrency = flag.Int("guest-concurrency", 8, "max concurrent usage requests")
		guestTimeout     = flag.Duration("guest-timeout", 25*time.Second, "per-guest usage timeout")
		onlyNamePrefix   = flag.String("only-name-prefix", "NR-", "only scrape servers with this name prefix")
		insecureSkipTLS  = flag.Bool("insecure-skip-tls", false, "skip TLS verification (lab only)")
	)
	flag.Parse()

	log.Logger = zerolog.New(os.Stdout).With().
		Timestamp().
		Str("service", "prometheus-solusvm-exporter").
		Logger()

	token, err := resolveToken(*svmTokenFile)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to resolve SolusVM token")
	}
	if *svmURL == "" {
		log.Fatal().Msg("--solusvm-url or SOLUSVM_URL is required")
	}

	client := svmclient.New(*svmURL, token, *insecureSkipTLS)

	snap := cache.NewSnapshot()

	invCol := collect.NewInventoryCollector(client, *onlyNamePrefix)
	capCol := collect.NewCapacityCollector(client)
	guestCol := collect.NewGuestCollector(client, *guestConcurrency, *guestTimeout, *onlyNamePrefix)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var lastServers []collect.ServerSummary
	var serversMu sync.Mutex
	var inventoryOK atomic.Bool

	runInventory := func() {
		families, servers, err := invCol.Collect(ctx)
		if err != nil {
			log.Error().Err(err).Msg("inventory collection failed")
			inventoryOK.Store(false)
		} else {
			inventoryOK.Store(true)
			serversMu.Lock()
			lastServers = servers
			serversMu.Unlock()
		}
		mergeSnapshot(snap, "inventory", families)
	}

	runCapacity := func() {
		families, err := capCol.Collect(ctx)
		if err != nil {
			log.Error().Err(err).Msg("capacity collection failed")
		}
		mergeSnapshot(snap, "capacity", families)
	}

	runGuest := func() {
		if !inventoryOK.Load() {
			log.Warn().Msg("skip guest scrape: last inventory failed")
			return
		}
		serversMu.Lock()
		srv := lastServers
		serversMu.Unlock()
		families := guestCol.Collect(ctx, srv)
		mergeSnapshot(snap, "guest", families)
	}

	runInventory()
	runCapacity()
	runGuest()

	go ticker(ctx, *refreshInventory, runInventory)
	go ticker(ctx, *refreshCapacity, runCapacity)
	go ticker(ctx, *refreshGuest, runGuest)

	mux := svmhttp.NewMux(snap)
	srv := &http.Server{
		Addr:         *listen,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info().Str("addr", *listen).Msg("starting exporter")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("http server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down")
	cancel()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error().Err(err).Msg("server shutdown error")
	}
	log.Info().Msg("exporter stopped")
}

func resolveToken(tokenFile string) (string, error) {
	if tokenFile != "" {
		data, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", fmt.Errorf("read token file: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	token := os.Getenv("SOLUSVM_TOKEN")
	if token == "" {
		return "", fmt.Errorf("set --solusvm-token-file or SOLUSVM_TOKEN")
	}
	return token, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func ticker(ctx context.Context, interval time.Duration, fn func()) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn()
		}
	}
}

var (
	snapshotParts   = map[string][]*dto.MetricFamily{}
	snapshotPartsMu sync.Mutex
)

func mergeSnapshot(snap *cache.Snapshot, name string, families []*dto.MetricFamily) {
	snapshotPartsMu.Lock()
	defer snapshotPartsMu.Unlock()

	snapshotParts[name] = families

	var merged []*dto.MetricFamily
	for _, fams := range snapshotParts {
		merged = append(merged, fams...)
	}
	snap.Update(merged)
}
