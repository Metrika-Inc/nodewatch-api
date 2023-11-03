// Copyright 2021 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package crawl

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"eth2-crawler/crawler/p2p"
	reqresp "eth2-crawler/crawler/rpc/request"
	"eth2-crawler/crawler/util"
	"eth2-crawler/graph/model"
	"eth2-crawler/models"
	"eth2-crawler/output"
	ipResolver "eth2-crawler/resolver"
	"eth2-crawler/store/peerstore"
	"eth2-crawler/store/record"
	"eth2-crawler/utils/config"
	uc "eth2-crawler/utils/crypto"
	"eth2-crawler/utils/fork"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/muxer/mplex"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	noise "github.com/libp2p/go-libp2p/p2p/security/noise"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/protolambda/zrnt/eth2/beacon/common"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

type crawler struct {
	crawlerConfig *config.Crawler
	disc          resolver
	peerStore     peerstore.Provider
	historyStore  record.Provider
	ipResolver    ipResolver.Provider
	iter          enode.Iterator
	nodeCh        chan *enode.Node
	host          p2p.Host
	pubSub        *pubsub.PubSub
	jobs          chan *models.Peer
	fileOutput    *output.Output
	// decoder       *beacon.ForkDecoder
	fockChoice *fork.ForkChoice
	hostLock   sync.RWMutex
}

// resolver holds methods of discovery v5
type resolver interface {
	Ping(n *enode.Node) error
}

// newCrawler inits new crawler service
func newCrawler(ctx context.Context, config *config.Crawler, disc resolver, peerStore peerstore.Provider, historyStore record.Provider,
	ipResolver ipResolver.Provider, iter enode.Iterator,
	fileOutput *output.Output, forkChoice *fork.ForkChoice) *crawler {

	host, err := newHost(ctx, forkChoice)
	if err != nil {
		log.Error("failed create new host", log.Ctx{"err": err})
		panic(101)
	}

	gs, err := startGossipSub(ctx, host)
	if err != nil {
		log.Error("failed start ", log.Ctx{"err": err})
		panic(102)
	}

	c := &crawler{
		disc:          disc,
		peerStore:     peerStore,
		historyStore:  historyStore,
		ipResolver:    ipResolver,
		iter:          iter,
		nodeCh:        make(chan *enode.Node),
		host:          host,
		pubSub:        gs,
		jobs:          make(chan *models.Peer, config.Concurrency),
		fileOutput:    fileOutput,
		crawlerConfig: config,
		// decoder:       config.ForkDecoder,
		fockChoice: forkChoice,
	}
	return c
}

func newHost(ctx context.Context, forkChoice *fork.ForkChoice) (p2p.Host, error) {
	pkey, err := crypto.GenerateKey()
	if err != nil {
		log.Error("failed generate key", log.Ctx{"err": err})
		return nil, err
	}

	cpkey, err := uc.ConvertToInterfacePrivkey(pkey)
	if err != nil {
		log.Error("failed convert key", log.Ctx{"err": err})
		return nil, err
	}

	listenAddrs, err := multiAddressBuilder(net.IPv4zero, 30304)
	if err != nil {
		return nil, err
	}
	host, err := p2p.NewHost(
		ctx,
		forkChoice,
		libp2p.Identity(cpkey),
		libp2p.ListenAddrs(listenAddrs),
		libp2p.UserAgent("Eth2-Crawler"),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.ChainOptions(libp2p.DefaultMuxers, libp2p.Muxer(mplex.ID, mplex.DefaultTransport)),
		libp2p.NATPortMap(),
	)
	if err != nil {
		log.Error("failed create new host", log.Ctx{"err": err})
		return nil, err
	}

	log.Info("---------------")
	log.Info("host created", log.Ctx{"host": host.ID().String()})
	log.Info("---------------")

	return host, nil
}

// start runs the crawler
func (c *crawler) start(ctx context.Context) {
	doneCh := make(chan enode.Iterator)
	go c.runIterator(ctx, doneCh, c.iter)
	for {
		select {
		case n := <-c.nodeCh:
			c.storePeer(ctx, n)
		case <-doneCh:
			// crawling finished
			log.Info("finished iterator")
			return
		}
	}
}

// runIterator uses the node iterator and sends node data through channel
func (c *crawler) runIterator(ctx context.Context, doneCh chan enode.Iterator, it enode.Iterator) {
	defer func() { doneCh <- it }()
	for it.Next() {
		select {
		case c.nodeCh <- it.Node():
		case <-ctx.Done():
			return
		}
	}
}

func (c *crawler) storePeer(ctx context.Context, node *enode.Node) {
	// only consider the node having tcp port exported
	if node.TCP() == 0 {
		return
	}
	// filter only eth2 nodes
	eth2Data, err := util.ParseEnrEth2Data(node)
	if err != nil { // not eth2 nodes
		return
	}

	if eth2Data.ForkDigest == c.fockChoice.Fork() {
		log.Debug("found a eth2 node", log.Ctx{"node": node})
		// get basic info
		peer, err := models.NewPeer(node, eth2Data)
		if err != nil {
			return
		}
		// save to db if not exists
		err = c.peerStore.Create(ctx, peer)
		if err != nil {
			log.Error("err inserting peer", log.Ctx{"err": err, "peer": peer.String()})
		}
	}
}

func (c *crawler) updatePeer(ctx context.Context, wg *sync.WaitGroup) {
	var bgWorkerWg sync.WaitGroup
	c.runBGWorkersPool(ctx, &bgWorkerWg)
	for {
		select {
		case <-ctx.Done():
			log.Error("update peer job context was canceled", log.Ctx{"err": ctx.Err()})

			// Wait for the worker pool to finish, then safe to close the write channel
			bgWorkerWg.Wait()
			close(c.fileOutput.WorkChan())

			wg.Done()
			return
		default:
			c.selectPendingAndExecute(ctx)
		}
		time.Sleep(5 * time.Second)
	}
}

func (c *crawler) selectPendingAndExecute(ctx context.Context) {
	// get peers updated more than config.UpdateFreqMin min ago
	reqs, err := c.peerStore.ListForJob(ctx, time.Minute*time.Duration(c.crawlerConfig.UpdateFreqMin), c.crawlerConfig.Concurrency)
	if err != nil {
		log.Error("error getting list from peerstore", log.Ctx{"err": err})
		return
	}
	for _, req := range reqs {
		// update the peer, so it won't be picked again in 24 hours
		// We have to update the LastUpdated field here and cannot rety on the worker to update it
		// That is because the same request will be picked again when it is in worker.
		req.LastUpdated = time.Now().Unix()
		err = c.peerStore.Update(ctx, req)
		if err != nil {
			log.Error("error updating request", log.Ctx{"err": err})
			continue
		}
		select {
		case <-ctx.Done():
			log.Error("update selector stopped", log.Ctx{"err": ctx.Err()})
			return
		default:
			c.jobs <- req
		}
	}
}

func (c *crawler) runBGWorkersPool(ctx context.Context, wg *sync.WaitGroup) {
	for i := 0; i < c.crawlerConfig.Concurrency; i++ {
		wg.Add(1)
		go c.bgWorker(ctx, wg)
	}
}

func (c *crawler) bgWorker(ctx context.Context, wg *sync.WaitGroup) {
	for {
		select {
		case <-ctx.Done():
			log.Error("context canceled", log.Ctx{"err": ctx.Err()})
			wg.Done()
			return
		case req := <-c.jobs:
			c.updatePeerInfo(ctx, req)
		}
	}
}

func (c *crawler) updatePeerInfo(ctx context.Context, peer *models.Peer) {
	// update connection status, agent version, sync status
	isConnectable := c.collectNodeInfoRetryer(ctx, peer)
	if isConnectable {
		peer.SetConnectionStatus(true)
		peer.Score = models.ScoreGood
		peer.LastConnected = time.Now().Unix()
		// update geolocation

		// if peer.GeoLocation == nil {
		// 	c.updateGeolocation(ctx, peer)
		// }

		h := sha256.New()
		h.Write([]byte(peer.ID))

		lastUpdateBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(lastUpdateBytes, uint64(peer.LastUpdated))
		h.Write(lastUpdateBytes)
		uuid := fmt.Sprintf("%x", h.Sum(nil))

		// TODO: Can we do anything here if this is blocking?
		c.fileOutput.WorkChan() <- models.PeerOutput{UUID: uuid, ProcessedTimestamp: time.Now().UTC(), Peer: *peer}
	} else {
		peer.Score--
	}
	// remove the node if it has bad score
	if peer.Score <= models.ScoreBad {
		log.Info("deleting node for bad score", log.Ctx{"peer_id": peer.ID})
		err := c.peerStore.Delete(ctx, peer)
		if err != nil {
			log.Error("failed on deleting from peerstore", log.Ctx{"err": err})
		}
		return
	}
	err := c.peerStore.Update(ctx, peer)
	if err != nil {
		log.Error("failed on updating peerstore", log.Ctx{"err": err})
	}
}

func (c *crawler) collectNodeInfoRetryer(ctx context.Context, peer *models.Peer) bool {
	c.hostLock.RLock()
	defer c.hostLock.RUnlock()
	count := 0
	var err error
	var ag, pv string
	for count < c.crawlerConfig.ConnectionRetries {

		select {
		case <-ctx.Done():
			log.Info("exiting node retryer, context done")
			return false
		case <-time.After(time.Second * 5):
			count++

			err = c.host.Connect(ctx, *peer.GetPeerInfo())
			if err != nil {
				switch t := err.(type) {
				default:
					log.Error("unknown type %T\n", t)
				case *swarm.DialError:
					for _, e := range t.DialErrors {
						// Bit ugly but failing to negotiate security protocol is a good indicator that the node will never be able to connect
						if strings.HasPrefix(e.Cause.Error(), "failed to negotiate security protocol") {
							log.Debug("failed to negotiate security protocol, removing from DB", log.Ctx{"peer": peer.ID.String()})
							peer.Score = models.ScoreBad
							break
						}
					}
				}

				continue
			}
			// get status
			var status *common.Status
			status, err = c.host.FetchStatus(c.host.NewStream, ctx, peer, new(reqresp.SnappyCompression))
			if err != nil || status == nil {
				log.Debug("Non-success result fetching status", log.Ctx{"err": err})
				continue
			}
			ag, err = c.host.GetAgentVersion(peer.ID)
			if err != nil {
				continue
			} else {
				peer.SetUserAgent(ag)
			}

			pv, err = c.host.GetProtocolVersion(peer.ID)
			if err != nil {
				continue
			} else {
				peer.SetProtocolVersion(pv)
			}

			// set sync status
			peer.SetSyncStatus(int64(status.HeadSlot))

			// Set the fork digest if it has changed
			peer.SetForkDigest(status.ForkDigest)

			log.Info("successfully collected all info", peer.Log())
			return true
		}
	}
	// unsuccessful
	log.Error("failed on retryer", log.Ctx{
		"attempt": count,
		"error":   err,
	})
	return false
}

func (c *crawler) updateGeolocation(ctx context.Context, peer *models.Peer) {
	geoLoc, err := c.ipResolver.GetGeoLocation(ctx, peer.IP)
	if err != nil {
		log.Error("unable to get geo information", log.Ctx{
			"error":   err,
			"ip_addr": peer.IP,
		})
		return
	}
	peer.SetGeoLocation(geoLoc)
}

func (c *crawler) insertToHistory() {
	ctx := context.Background()
	// get count
	aggregateData, err := c.peerStore.AggregateBySyncStatus(ctx, &model.PeerFilter{})
	if err != nil {
		log.Error("error getting sync status", log.Ctx{"err": err})
	}

	history := models.NewHistory(aggregateData.Synced, aggregateData.Total)
	err = c.historyStore.Create(ctx, history)
	if err != nil {
		log.Error("error inserting sync status", log.Ctx{"err": err})
	}
}
