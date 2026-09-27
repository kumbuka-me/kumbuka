package metrics

import (
	"github.com/kumbuka-me/kumbuka/internal/postgres"
	"github.com/prometheus/client_golang/prometheus"
)

// postgresStatsProvider supplies one point-in-time PostgreSQL pool snapshot.
type postgresStatsProvider interface {
	PoolStats() postgres.PoolStats
}

// postgresCollector exports PostgreSQL connection-pool state.
type postgresCollector struct {
	// provider supplies the current PostgreSQL pool statistics at scrape time.
	provider postgresStatsProvider
	// connections describes current pool connections partitioned by state.
	connections *prometheus.Desc
	// maxConnections describes the configured PostgreSQL pool connection limit.
	maxConnections *prometheus.Desc
	// acquires describes successful PostgreSQL pool acquisitions.
	acquires *prometheus.Desc
	// emptyAcquires describes successful acquisitions that had to wait for capacity.
	emptyAcquires *prometheus.Desc
	// canceledAcquires describes acquisitions canceled by their context.
	canceledAcquires *prometheus.Desc
	// acquireDuration describes cumulative successful acquisition wait time.
	acquireDuration *prometheus.Desc
	// emptyAcquireWaitTime describes cumulative wait time caused by an empty pool.
	emptyAcquireWaitTime *prometheus.Desc
	// newConnections describes connections opened by the PostgreSQL pool.
	newConnections *prometheus.Desc
	// destroyedConnections describes pool connections destroyed by reason.
	destroyedConnections *prometheus.Desc
}

// RegisterPostgres adds scrape-time PostgreSQL connection-pool metrics to the private registry.
func (r *Registry) RegisterPostgres(database *postgres.Store) {
	if !r.enabled {
		return
	}

	r.registry.MustRegister(newPostgresCollector(database))
}

// newPostgresCollector constructs the Prometheus collector around a PostgreSQL pool-stat provider.
func newPostgresCollector(provider postgresStatsProvider) *postgresCollector {
	return &postgresCollector{
		provider: provider,
		connections: prometheus.NewDesc(
			"kumbuka_postgres_pool_connections",
			"Current PostgreSQL connection-pool connections by state.",
			[]string{"state"},
			nil,
		),
		maxConnections: prometheus.NewDesc(
			"kumbuka_postgres_pool_max_connections",
			"Maximum number of connections allowed in the PostgreSQL connection pool.",
			nil,
			nil,
		),
		acquires: prometheus.NewDesc(
			"kumbuka_postgres_pool_acquires_total",
			"Total number of successful PostgreSQL connection-pool acquisitions.",
			nil,
			nil,
		),
		emptyAcquires: prometheus.NewDesc(
			"kumbuka_postgres_pool_empty_acquires_total",
			"Total successful acquisitions that waited because the PostgreSQL connection pool was empty.",
			nil,
			nil,
		),
		canceledAcquires: prometheus.NewDesc(
			"kumbuka_postgres_pool_canceled_acquires_total",
			"Total PostgreSQL connection-pool acquisitions canceled by their context.",
			nil,
			nil,
		),
		acquireDuration: prometheus.NewDesc(
			"kumbuka_postgres_pool_acquire_duration_seconds_total",
			"Total time spent successfully acquiring PostgreSQL connections from the pool.",
			nil,
			nil,
		),
		emptyAcquireWaitTime: prometheus.NewDesc(
			"kumbuka_postgres_pool_empty_acquire_wait_seconds_total",
			"Total time successful callers waited because the PostgreSQL connection pool was empty.",
			nil,
			nil,
		),
		newConnections: prometheus.NewDesc(
			"kumbuka_postgres_pool_new_connections_total",
			"Total number of PostgreSQL connections opened by the pool.",
			nil,
			nil,
		),
		destroyedConnections: prometheus.NewDesc(
			"kumbuka_postgres_pool_destroyed_connections_total",
			"Total PostgreSQL pool connections destroyed by reason.",
			[]string{"reason"},
			nil,
		),
	}
}

// Describe publishes the PostgreSQL pool metric descriptors.
func (c *postgresCollector) Describe(descriptions chan<- *prometheus.Desc) {
	descriptions <- c.connections
	descriptions <- c.maxConnections
	descriptions <- c.acquires
	descriptions <- c.emptyAcquires
	descriptions <- c.canceledAcquires
	descriptions <- c.acquireDuration
	descriptions <- c.emptyAcquireWaitTime
	descriptions <- c.newConnections
	descriptions <- c.destroyedConnections
}

// Collect snapshots the PostgreSQL pool once and publishes gauges and cumulative counters from that snapshot.
func (c *postgresCollector) Collect(metrics chan<- prometheus.Metric) {
	stats := c.provider.PoolStats()

	metrics <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(stats.AcquiredConnections), "acquired")
	metrics <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(stats.IdleConnections), "idle")
	metrics <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(stats.ConstructingConnections), "constructing")
	metrics <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(stats.TotalConnections), "total")
	metrics <- prometheus.MustNewConstMetric(c.maxConnections, prometheus.GaugeValue, float64(stats.MaxConnections))
	metrics <- prometheus.MustNewConstMetric(c.acquires, prometheus.CounterValue, float64(stats.Acquires))
	metrics <- prometheus.MustNewConstMetric(c.emptyAcquires, prometheus.CounterValue, float64(stats.EmptyAcquires))
	metrics <- prometheus.MustNewConstMetric(c.canceledAcquires, prometheus.CounterValue, float64(stats.CanceledAcquires))
	metrics <- prometheus.MustNewConstMetric(c.acquireDuration, prometheus.CounterValue, stats.AcquireDuration.Seconds())
	metrics <- prometheus.MustNewConstMetric(c.emptyAcquireWaitTime, prometheus.CounterValue, stats.EmptyAcquireWaitTime.Seconds())
	metrics <- prometheus.MustNewConstMetric(c.newConnections, prometheus.CounterValue, float64(stats.NewConnections))
	metrics <- prometheus.MustNewConstMetric(c.destroyedConnections, prometheus.CounterValue, float64(stats.MaxIdleDestroyed), "idle")
	metrics <- prometheus.MustNewConstMetric(c.destroyedConnections, prometheus.CounterValue, float64(stats.MaxLifetimeDestroyed), "max_lifetime")
}
