// Copyright 2021 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

// Package crawl holds the eth2 node discovery utilities
package crawl

import (
	"context"
	"crypto/ecdsa"
	"eth2-crawler/output"
	"eth2-crawler/store/peerstore"
	"eth2-crawler/store/record"
	clock "eth2-crawler/utils/clock"
	"eth2-crawler/utils/config"
	fork "eth2-crawler/utils/fork"
	"fmt"
	"net"
	"sync"

	"github.com/robfig/cron/v3"

	ipResolver "eth2-crawler/resolver"

	"github.com/ethereum/go-ethereum/crypto"
	ma "github.com/multiformats/go-multiaddr"
)

// listenConfig holds configuration for running v5discovry node
type listenConfig struct {
	bootNodeAddrs []string
	listenAddress net.IP
	listenPORT    int
	dbPath        string
	privateKey    *ecdsa.PrivateKey
}

// Initialize initializes the core crawler component
func Initialize(ctx context.Context, wg *sync.WaitGroup, config *config.Configuration, peerStore peerstore.Provider, historyStore record.Provider, ipResolver ipResolver.Provider, bootNodeAddrs []string, fileOutput *output.Output) error {
	pkey, _ := crypto.GenerateKey()
	listenCfg := &listenConfig{
		bootNodeAddrs: bootNodeAddrs,
		listenAddress: net.IPv4zero,
		listenPORT:    30304,
		dbPath:        "",
		privateKey:    pkey,
	}
	disc, err := startV5(listenCfg)
	if err != nil {
		return err
	}

	slotClock := clock.NewClock(config.Crawler.GenesisTime, config.Crawler.SecondsPerSlot, config.Crawler.SlotsPerEpoch)
	forkChoice := fork.NewForkChoice(ctx, slotClock, config.Fork)

	c := newCrawler(ctx, config.Crawler, disc, peerStore, historyStore, ipResolver, disc.RandomNodes(), fileOutput, forkChoice)
	go c.start(ctx)

	// scheduler for updating peer
	wg.Add(1)
	go c.updatePeer(ctx, wg)

	// Start file output
	wg.Add(1)
	go c.fileOutput.Start(ctx, wg)

	// add scheduler for updating history store
	scheduler := cron.New()
	_, err = scheduler.AddFunc("@daily", c.insertToHistory)
	if err != nil {
		return err
	}
	scheduler.Start()
	return nil
}

func multiAddressBuilder(ipAddr net.IP, port int) (ma.Multiaddr, error) {
	if ipAddr.To4() == nil && ipAddr.To16() == nil {
		return nil, fmt.Errorf("invalid ip address provided: %s", ipAddr)
	}
	if ipAddr.To4() != nil {
		return ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ipAddr.String(), port))
	}
	return ma.NewMultiaddr(fmt.Sprintf("/ip6/%s/tcp/%d", ipAddr.String(), port))
}
