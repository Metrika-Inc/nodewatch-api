// Copyright 2021 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

// Package crawler holds the whole crawler service. It includes crawler, db component and GraphQL
package crawler

import (
	"context"
	"eth2-crawler/crawler/crawl"
	"eth2-crawler/output"
	ipResolver "eth2-crawler/resolver"
	"eth2-crawler/store/peerstore"
	"eth2-crawler/store/record"
	"eth2-crawler/utils/config"
	"sync"

	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum/go-ethereum/params"
)

// Start starts the crawler service
func Start(ctx context.Context, wg *sync.WaitGroup, config *config.Crawler, peerStore peerstore.Provider, historyStore record.Provider, ipResolver ipResolver.Provider, fileOutput *output.Output) {
	h := log.CallerFileHandler(log.StdoutHandler)
	log.Root().SetHandler(h)

	handler := log.MultiHandler(
		log.LvlFilterHandler(log.LvlInfo, h),
	)
	log.Root().SetHandler(handler)

	err := crawl.Initialize(ctx, wg, config, peerStore, historyStore, ipResolver, params.V5Bootnodes, fileOutput)
	if err != nil {
		panic(err)
	}
}
