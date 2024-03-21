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
		Help:      "Counter tracking the raw number of ETH2 nodes with successful connections",
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

	NodeUpdateDelay = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "nodewatchCrawler",
		Name:      "node_connection_delay",
		Help:      "Histogram tracking the delay between adding a node to the jobs queue and successfully establishing a connection",
		Buckets:   prometheus.LinearBuckets(0, 10, 18),
	})

	KafkaWriteMessageDelay = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "nodewatchCrawler",
		Name:      "kafka_write_message_delay",
		Help:      "Histogram tracking kafka write latency",
		Buckets:   prometheus.DefBuckets,
	})
)
