package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	Up = prometheus.NewDesc("svm_up", "SolusVM 2 API reachable via GET /account", nil, nil)

	ComputeResourceUp = prometheus.NewDesc("svm_compute_resource_up",
		"Compute resource active and not locked",
		[]string{"compute_resource_id", "location_id", "name"}, nil)

	CPUAllocated = prometheus.NewDesc("svm_cpu_allocated",
		"vCPU allocated on this compute resource",
		[]string{"compute_resource_id"}, nil)
	CPUMax = prometheus.NewDesc("svm_cpu_max",
		"Max vCPU on this compute resource (+Inf when SolusVM limit is unlimited)",
		[]string{"compute_resource_id"}, nil)

	MemoryAllocatedBytes = prometheus.NewDesc("svm_memory_allocated_bytes",
		"Memory allocated on this compute resource (bytes)",
		[]string{"compute_resource_id"}, nil)
	MemoryMaxBytes = prometheus.NewDesc("svm_memory_max_bytes",
		"Max memory on this compute resource (bytes; +Inf when unlimited)",
		[]string{"compute_resource_id"}, nil)

	DiskAllocatedBytes = prometheus.NewDesc("svm_disk_allocated_bytes",
		"Disk allocated on this compute resource (bytes)",
		[]string{"compute_resource_id"}, nil)
	DiskMaxBytes = prometheus.NewDesc("svm_disk_max_bytes",
		"Max disk on this compute resource (bytes; +Inf when unlimited)",
		[]string{"compute_resource_id"}, nil)

	ServerInfo = prometheus.NewDesc("svm_server_info",
		"NodeRings SolusVM 2 server (name prefix NR-)",
		[]string{"name", "server_id", "uuid", "compute_resource_id"}, nil)

	GuestUp = prometheus.NewDesc("svm_guest_up",
		"1 if the NodeRings SolusVM 2 server is started and not suspended",
		[]string{"name", "server_id", "uuid"}, nil)

	CPUUsageRatio = prometheus.NewDesc("svm_cpu_usage_ratio",
		"CPU usage 0-1",
		[]string{"name", "server_id", "uuid"}, nil)

	MemoryUsageBytes = prometheus.NewDesc("svm_memory_usage_bytes",
		"Guest memory usage (bytes)",
		[]string{"name", "server_id", "uuid"}, nil)

	DiskReadBytes = prometheus.NewDesc("svm_disk_read_bytes",
		"Disk read rate (gauge)",
		[]string{"name", "server_id", "uuid"}, nil)
	DiskWriteBytes = prometheus.NewDesc("svm_disk_write_bytes",
		"Disk write rate (gauge)",
		[]string{"name", "server_id", "uuid"}, nil)

	NetworkReceiveBytes = prometheus.NewDesc("svm_network_receive_bytes",
		"Network receive rate (gauge)",
		[]string{"name", "server_id", "uuid"}, nil)
	NetworkTransmitBytes = prometheus.NewDesc("svm_network_transmit_bytes",
		"Network transmit rate (gauge)",
		[]string{"name", "server_id", "uuid"}, nil)

	GuestScrapeSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "svm_guest_scrape_seconds",
		Help:    "Time spent collecting guest usage metrics",
		Buckets: prometheus.DefBuckets,
	})

	UsageErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "svm_usage_errors_total",
		Help: "Errors fetching usage from SolusVM 2",
	}, []string{"reason"})

	ExporterLastSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "svm_exporter_last_success_timestamp",
		Help: "Unix timestamp of last successful collection",
	}, []string{"collector"})
)

func init() {
	prometheus.MustRegister(GuestScrapeSeconds)
	prometheus.MustRegister(UsageErrorsTotal)
	prometheus.MustRegister(ExporterLastSuccess)
}
