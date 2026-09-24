package metrics

import (
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/prometheus/client_golang/prometheus"
)

// PluginProvider supplies the current in-memory plugin lifecycle state.
type PluginProvider interface {
	// Plugins returns the current plugin metadata snapshot.
	Plugins() []plugin.LoadedPlugin
}

// pluginCollector exports aggregate installed and enabled plugin counts without per-plugin lifecycle labels.
type pluginCollector struct {
	// provider supplies a consistent plugin metadata snapshot for each scrape.
	provider PluginProvider
	// installed describes the total number of plugins known to the manager.
	installed *prometheus.Desc
	// enabled describes the total number of plugins currently contributing active modules.
	enabled *prometheus.Desc
}

// RegisterPluginProvider adds scrape-time plugin lifecycle gauges to the private registry.
func (r *Registry) RegisterPluginProvider(provider PluginProvider) {
	if provider == nil {
		return
	}

	r.registry.MustRegister(&pluginCollector{
		provider: provider,
		installed: prometheus.NewDesc(
			"kumbuka_plugins_installed",
			"Number of plugins currently known to the plugin manager.",
			nil,
			nil,
		),
		enabled: prometheus.NewDesc(
			"kumbuka_plugins_enabled",
			"Number of plugins currently enabled in the plugin manager.",
			nil,
			nil,
		),
	})
}

// Describe publishes the plugin lifecycle metric descriptors.
func (c *pluginCollector) Describe(descriptions chan<- *prometheus.Desc) {
	descriptions <- c.installed
	descriptions <- c.enabled
}

// Collect snapshots plugin lifecycle state and publishes aggregate gauges.
func (c *pluginCollector) Collect(metrics chan<- prometheus.Metric) {
	plugins := c.provider.Plugins()
	enabled := 0
	for _, item := range plugins {
		if item.Enabled {
			enabled++
		}
	}

	metrics <- prometheus.MustNewConstMetric(c.installed, prometheus.GaugeValue, float64(len(plugins)))
	metrics <- prometheus.MustNewConstMetric(c.enabled, prometheus.GaugeValue, float64(enabled))
}
