// Package metrics provides functionality for handling metrics.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	NodeDiscovered = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "nodewatchCrawler",
		Name:      "counter_node_discovered",
		Help:      "Counter tracking the raw number of ETH2 node records discovered",
	})

	NodeInfoCollected = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "nodewatchCrawler",
		Name:      "counter_info_collected",
		Help:      "Counter tracking the raw number of ETH2 nodes successfully connected to queried",
	})

	NodeInfoFailed = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "nodewatchCrawler",
		Name:      "counter_info_failed",
		Help:      "Counter tracking the raw number of failed node connection requests",
	})

	WriteChannelSize = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "nodewatchCrawler",
		Name:      "write_channel_size",
		Help:      "Gauge tracking the current length of the write channel",
	})

	NodeRemoved = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "nodewatchCrawler",
		Name:      "counter_node_removed",
		Help:      "Counter tracking the raw number of ETH2 node records removed from the crawler DB",
	})
)