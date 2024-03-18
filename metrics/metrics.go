// Package metrics provides functionality for handling metrics.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	NodeDiscovered = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "nodewatch-crawler",
		Name:      "counter_node_discovered",
		Help:      "Counter tracking the raw number of ETH2 node records discovered",
	})

	NodeInfoCollected = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "nodewatch-crawler",
		Name:      "counter_info_collected",
		Help:      "Counter tracking the raw number of ETH2 nodes successfully connected to queried",
	})
)
