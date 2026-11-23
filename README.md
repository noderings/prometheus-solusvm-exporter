# prometheus-solusvm-exporter

A [Prometheus](https://prometheus.io/) exporter for [SolusVM 2](https://www.solusvm.com/). It reads compute resources, capacity and guest usage from the SolusVM 2 Admin API and exposes them as `svm_*` metrics.

It was built for [NodeRings](https://noderings.com/), but it works with any SolusVM 2 management node.

> SolusVM 1 (port `5656`, `/api/admin`) is not supported.

## Features

- Capacity per compute resource: allocated vs. maximum vCPU, memory and disk. A limit set to "unlimited" is exported as `+Inf`.
- Guest CPU, memory, disk and network usage from the SolusVM usage endpoints.
- Scrapes never hit the API. Background workers refresh an in-memory snapshot, and `/metrics` only serves that cache.
- Ships as a single static binary or a multi-arch (amd64/arm64) container image that runs as a non-root user.

## Quick start

### Docker

```bash
docker run -d -p 9179:9179 \
  -e SOLUSVM_URL=https://mn.example.com \
  -e SOLUSVM_TOKEN=<api-token> \
  harbor.noderings.com/nrings/prometheus-solusvm-exporter:latest
```

### Binary

```bash
make build
./prometheus-solusvm-exporter \
  --solusvm-url=https://mn.example.com \
  --solusvm-token-file=/var/run/secrets/svm/token
```

Create the API token in SolusVM 2 under **Account → API Tokens**. The exporter only sends `GET` requests.

### Prometheus

```yaml
scrape_configs:
  - job_name: solusvm
    static_configs:
      - targets: ["exporter-host:9179"]
```

## Configuration

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--listen` | `LISTEN_ADDR` | `:9179` | Listen address |
| `--solusvm-url` | `SOLUSVM_URL` | | Management node URL, without `/api/v1` |
| `--solusvm-token-file` | | | File holding the API bearer token |
| | `SOLUSVM_TOKEN` | | API bearer token. `--solusvm-token-file` takes precedence. |
| `--refresh-inventory` | | `30s` | Inventory refresh interval |
| `--refresh-capacity` | | `60s` | Capacity refresh interval |
| `--refresh-guest` | | `45s` | Guest metrics refresh interval |
| `--guest-concurrency` | | `8` | Maximum concurrent usage requests |
| `--guest-timeout` | | `25s` | Timeout for each guest usage request |
| `--only-name-prefix` | | `NR-` | Only export per-guest series for servers whose name starts with this prefix (case-insensitive). Set it to `""` to export every guest. |
| `--insecure-skip-tls` | | `false` | Skip TLS verification (testing only) |

## Endpoints

| Path | Description |
|------|-------------|
| `/metrics` | Prometheus metrics, served from the in-memory snapshot |
| `/healthz` | Liveness check |

## How it works

| Worker | Interval | SolusVM 2 API calls |
|--------|----------|---------------------|
| inventory | 30s | `GET /account`, `GET /compute_resources`, `GET /servers` (paginated) |
| capacity | 60s | Compute resource `settings.limits` (vCPU, RAM, disk) |
| guest | 45s | `GET /usage/{cpu,memory,network,disks}/{uuid}` for each running guest that matches the name prefix |

## Metrics

### Compute resources and capacity

| Metric | Labels | Description |
|--------|--------|-------------|
| `svm_up` | | 1 when `GET /account` succeeds |
| `svm_compute_resource_up` | `compute_resource_id`, `location_id`, `name` | Compute resource is active and not locked |
| `svm_cpu_allocated` / `svm_cpu_max` | `compute_resource_id` | vCPUs sold / sellable |
| `svm_memory_allocated_bytes` / `svm_memory_max_bytes` | `compute_resource_id` | Memory sold / sellable |
| `svm_disk_allocated_bytes` / `svm_disk_max_bytes` | `compute_resource_id` | Disk sold / sellable |

### Guests

| Metric | Labels | Description |
|--------|--------|-------------|
| `svm_server_info` | `name`, `server_id`, `uuid`, `compute_resource_id` | Maps a guest to its compute resource |
| `svm_guest_up` | `name`, `server_id`, `uuid` | 1 when the guest is started and not suspended |
| `svm_cpu_usage_ratio` | `name`, `server_id`, `uuid` | CPU usage, 0 to 1 |
| `svm_memory_usage_bytes` | `name`, `server_id`, `uuid` | Memory used |
| `svm_disk_read_bytes` / `svm_disk_write_bytes` | `name`, `server_id`, `uuid` | Disk throughput (gauge, as reported by SolusVM) |
| `svm_network_receive_bytes` / `svm_network_transmit_bytes` | `name`, `server_id`, `uuid` | Network throughput (gauge, as reported by SolusVM) |

### Exporter

| Metric | Labels | Description |
|--------|--------|-------------|
| `svm_guest_scrape_seconds` | | Time spent collecting guest metrics (histogram) |
| `svm_usage_errors_total` | `reason` | Failed usage fetches |
| `svm_exporter_last_success_timestamp` | `collector` | Unix time of each worker's last successful run |

## Development

```bash
make test        # go test ./...
make test-race   # go test -race
make lint        # golangci-lint
make build       # static binary
make docker-push IMG=registry.example.com/you/prometheus-solusvm-exporter TAG=dev
```

Requires Go 1.25+. Pushes to `main` build and publish the container image through GitHub Actions.

## License

Apache License 2.0. See [LICENSE](LICENSE).
