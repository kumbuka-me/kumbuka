package metrics

import (
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/postgres"
	"github.com/stretchr/testify/assert"
)

// postgresStatsProviderStub supplies deterministic pool statistics to the collector.
type postgresStatsProviderStub struct {
	stats postgres.PoolStats
}

// PoolStats returns the configured point-in-time pool snapshot.
func (s postgresStatsProviderStub) PoolStats() postgres.PoolStats { return s.stats }

func TestRegisterPostgresDisabledRegistryIsNoop(t *testing.T) {
	t.Parallel()

	registry := NewRegistry(false, "test", "test")
	registry.RegisterPostgres(nil)

	assert.False(t, registry.enabled)
}

func TestPostgresCollectorExportsPoolStats(t *testing.T) {
	t.Parallel()

	registry := NewRegistry(true, "test", "test")
	registry.registry.MustRegister(newPostgresCollector(postgresStatsProviderStub{stats: postgres.PoolStats{
		AcquiredConnections:     12,
		IdleConnections:         3,
		ConstructingConnections: 2,
		TotalConnections:        17,
		MaxConnections:          32,
		Acquires:                1200,
		EmptyAcquires:           41,
		CanceledAcquires:        7,
		AcquireDuration:         2500 * time.Millisecond,
		EmptyAcquireWaitTime:    1750 * time.Millisecond,
		NewConnections:          44,
		MaxIdleDestroyed:        5,
		MaxLifetimeDestroyed:    6,
	}}))

	output := scrapeMetrics(t, registry)
	assert.Contains(t, output, `kumbuka_postgres_pool_connections{state="acquired"} 12`)
	assert.Contains(t, output, `kumbuka_postgres_pool_connections{state="idle"} 3`)
	assert.Contains(t, output, `kumbuka_postgres_pool_connections{state="constructing"} 2`)
	assert.Contains(t, output, `kumbuka_postgres_pool_connections{state="total"} 17`)
	assert.Contains(t, output, "kumbuka_postgres_pool_max_connections 32")
	assert.Contains(t, output, "kumbuka_postgres_pool_acquires_total 1200")
	assert.Contains(t, output, "kumbuka_postgres_pool_empty_acquires_total 41")
	assert.Contains(t, output, "kumbuka_postgres_pool_canceled_acquires_total 7")
	assert.Contains(t, output, "kumbuka_postgres_pool_acquire_duration_seconds_total 2.5")
	assert.Contains(t, output, "kumbuka_postgres_pool_empty_acquire_wait_seconds_total 1.75")
	assert.Contains(t, output, "kumbuka_postgres_pool_new_connections_total 44")
	assert.Contains(t, output, `kumbuka_postgres_pool_destroyed_connections_total{reason="idle"} 5`)
	assert.Contains(t, output, `kumbuka_postgres_pool_destroyed_connections_total{reason="max_lifetime"} 6`)
}
