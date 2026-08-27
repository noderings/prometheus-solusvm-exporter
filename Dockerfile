# syntax=docker/dockerfile:1

##############################################
# Builder Stage
##############################################
FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.25.4-alpine AS builder

ARG TARGETPLATFORM BUILDPLATFORM TARGETOS TARGETARCH

RUN apk update && apk add --no-cache make git

WORKDIR /code
COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -buildvcs=false \
    -trimpath \
    -ldflags "-s -w" \
    -o prometheus-solusvm-exporter \
    ./cmd/exporter

##############################################
# Runtime Stage
##############################################
FROM alpine:3.23 AS runtime

WORKDIR /app

COPY --from=builder /code/prometheus-solusvm-exporter /app/prometheus-solusvm-exporter

RUN apk update && apk add --no-cache ca-certificates

RUN addgroup -g 1000 exporter && \
    adduser -D -u 1000 -G exporter exporter && \
    chown -R exporter:exporter /app

USER exporter

EXPOSE 9179

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9179/healthz || exit 1

ENTRYPOINT ["/app/prometheus-solusvm-exporter"]
