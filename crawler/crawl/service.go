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
	"eth2-crawler/utils/config"
	"fmt"
	"net"
	"sync"

	"github.com/robfig/cron/v3"

	ipResolver "eth2-crawler/resolver"

	"github.com/ethereum/go-ethereum/crypto"
	ic "github.com/libp2p/go-libp2p-core/crypto"
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
func Initialize(ctx context.Context, wg *sync.WaitGroup, config *config.Crawler, peerStore peerstore.Provider, historyStore record.Provider, ipResolver ipResolver.Provider, bootNodeAddrs []string, fileOutput *output.FileOutput) error {
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

	c := newCrawler(config, disc, peerStore, historyStore, ipResolver, listenCfg.privateKey, disc.RandomNodes(), fileOutput)
	go c.start(ctx)

	// scheduler for updating peer
	wg.Add(1)
	go c.updatePeer(ctx, wg)

	// Start file output
	wg.Add(1)
	go c.fileOutput.Start(ctx, wg)

	// Begin host refresh time
	wg.Add(1)
	go c.updateHost(ctx, wg)

	// add scheduler for updating history store
	scheduler := cron.New()
	_, err = scheduler.AddFunc("@daily", c.insertToHistory)
	if err != nil {
		return err
	}
	scheduler.Start()
	return nil
}

func convertToInterfacePrivkey(privkey *ecdsa.PrivateKey) ic.PrivKey {
	typeAssertedKey := ic.PrivKey((*ic.Secp256k1PrivateKey)(privkey))
	return typeAssertedKey
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
