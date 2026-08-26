# prometheus-solusvm-exporter

Prometheus exporter for SolusVM 2 hypervisor metrics. Runs on the NodeRings provider agent, reads inventory and guest usage from the SolusVM 2 Admin API, and exposes `svm_*` series on `:9179/metrics`.

SolusVM 1 (`:5656`, `/api/admin`) is not supported.

## Architecture

Background workers refresh an in-memory snapshot. The `/metrics` handler **never** calls the SolusVM API — it only serialises the cached snapshot.

| Worker | Default interval | SVM endpoint |
|--------|-----------------|--------------|
| inventory | 30 s | `GET /account`, paginated `GET /compute_resources`, paginated `GET /servers`. Emits `svm_server_info` for `NR-` names only. |
| capacity | 60 s | Compute-resource `settings.limits` (vcpu / ram / hdd used vs total). |
| guest | 45 s | `GET /usage/cpu\|memory\|network\|disks/{uuid}` for **running NR-** servers (skipped if last inventory failed). Empty `items` (no qemu-guest-agent) are tolerated. `GET /servers/{id}` `usage.cpu` is a fallback. |

Guest workers scrape **only** servers whose name starts with `NR-` (case-insensitive) **and** look started. `svm_server_info` is also NR-only.

Network/disk WHMCS `derivative.read_kb` / `write_kb` are rates; they are emitted as **gauges** until a lab dump confirms counters.

## Usage

```bash
prometheus-solusvm-exporter \
  --solusvm-url=https://mn.example.com \
  --solusvm-token-file=/var/run/secrets/svm/token \
  --listen=:9179
```

Or via environment variables:

```bash
export SOLUSVM_URL=https://mn.example.com
export SOLUSVM_TOKEN=<token>
prometheus-solusvm-exporter
```

## Endpoints

| Path | Description |
|------|-------------|
| `/metrics` | Prometheus metrics (reads in-memory snapshot only) |
| `/healthz` | Health check — process up |

## Configuration

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--listen` | `LISTEN_ADDR` | `:9179` | Listen address |
| `--solusvm-url` | `SOLUSVM_URL` | | Management node URL (no `/api/v1`) |
| `--solusvm-token-file` | | | Path to file containing bearer token |
| | `SOLUSVM_TOKEN` | | Bearer token (flag file takes precedence) |
| `--refresh-inventory` | | `30s` | Inventory refresh interval |
| `--refresh-capacity` | | `60s` | Capacity refresh interval |
| `--refresh-guest` | | `45s` | Guest metrics refresh interval |
| `--guest-concurrency` | | `8` | Max concurrent usage requests |
| `--guest-timeout` | | `25s` | Per-guest usage timeout |
| `--only-name-prefix` | | `NR-` | Only emit guest series for names with this prefix (case-insensitive) |
| `--insecure-skip-tls` | | `false` | Skip TLS verification (lab only) |

## Testing

```bash
go test -v ./...
go test -race -count=1 ./...
```

## Image

`harbor.noderings.com/noderings/prometheus-solusvm-exporter`
