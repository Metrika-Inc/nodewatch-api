// Copyright 2021 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

// Package p2p represent p2p host service
package p2p

import (
	"context"
	"encoding/hex"
	"errors"
	"eth2-crawler/crawler/rpc/methods"
	reqresp "eth2-crawler/crawler/rpc/request"
	"eth2-crawler/models"
	"eth2-crawler/utils/fork"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/protolambda/zrnt/eth2/beacon/common"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/identify"
)

// Client represent custom p2p client
type Client struct {
	ctx context.Context
	host.Host
	idSvc idService

	// node's metadata in the network
	LocalStatus   common.Status
	LocalMetadata common.MetaData
	ForkChoice    *fork.ForkChoice
}

func newClient(ctx context.Context, h host.Host, idSvc idService, forkChoice *fork.ForkChoice) *Client {
	c := &Client{
		ctx:           ctx,
		LocalStatus:   newLocalStatus(forkChoice.Fork()),
		ForkChoice:    forkChoice,
		LocalMetadata: newMetadata(),
		Host:          h,
		idSvc:         idSvc,
	}

	c.ServeBeaconMetadata()
	c.ServeBeaconPing()
	c.ServeBeaconStatus()
	return c
}

func newLocalStatus(forkDigest common.ForkDigest) common.Status {
	return common.Status{
		ForkDigest:     forkDigest,
		FinalizedRoot:  common.Root{},
		FinalizedEpoch: 0,
		HeadRoot:       common.Root{},
		HeadSlot:       0,
	}
}

func newMetadata() common.MetaData {
	attnets := new(common.AttnetBits)
	b, err := hex.DecodeString("ffffffffffffffff")
	if err != nil {
		log.Error("unable to decode Attnets", err.Error())
	}
	attnets.UnmarshalText(b)

	return common.MetaData{
		SeqNumber: 0,
		Attnets:   *attnets,
	}
}

const (
	RPCTimeout time.Duration = 20 * time.Second
)

// Host represent p2p services
type Host interface {
	host.Host
	IdentifyRequest(ctx context.Context, peerInfo *peer.AddrInfo) error
	GetProtocolVersion(peer.ID) (string, error)
	GetAgentVersion(peer.ID) (string, error)
	FetchStatus(sFn reqresp.NewStreamFn, ctx context.Context, peer *models.Peer, comp reqresp.Compression) (
		*common.Status, error)

	// Serve min number of endpoints we need to be a valid peer
	ServeBeaconPing()
	ServeBeaconStatus()
	ServeBeaconMetadata()
}

type idService interface {
	IdentifyWait(c network.Conn) <-chan struct{}
}

// NewHost initializes custom host
func NewHost(ctx context.Context, forkChoice *fork.ForkChoice, opt ...libp2p.Option) (Host, error) {
	h, err := libp2p.New(opt...)
	if err != nil {
		return nil, err
	}
	idService, err := identify.NewIDService(h)
	if err != nil {
		return nil, err
	}
	return newClient(ctx, h, idService, forkChoice), nil
}

// IdentifyRequest performs libp2p identify request after connecting to peer.
// It disconnects to peer after request is done
func (c *Client) IdentifyRequest(ctx context.Context, peerInfo *peer.AddrInfo) error {
	// Connect to peer first
	err := c.Connect(ctx, *peerInfo)
	if err != nil {
		return fmt.Errorf("error connecting to peer: %w", err)
	}
	defer func() {
		_ = c.Network().ClosePeer(peerInfo.ID)
	}()
	if conns := c.Network().ConnsToPeer(peerInfo.ID); len(conns) > 0 {
		select {
		case <-c.idSvc.IdentifyWait(conns[0]):
		case <-ctx.Done():
		}
	} else {
		return errors.New("not connected to peer, cannot await connection identify")
	}
	return nil
}

// GetProtocolVersion returns peer protocol version from peerstore.
// Need to call IdentifyRequest first for a peer.
func (c *Client) GetProtocolVersion(peerID peer.ID) (string, error) {
	key := "ProtocolVersion"
	value, err := c.Peerstore().Get(peerID, key)
	if err != nil {
		return "", fmt.Errorf("error getting protocol version:%w", err)
	}
	version, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("error converting interface to string")
	}
	return version, nil
}

// GetAgentVersion returns peer agent version  from peerstore.
// Need to call IdentifyRequest first for a peer.
func (c *Client) GetAgentVersion(peerID peer.ID) (string, error) {
	key := "AgentVersion"
	value, err := c.Peerstore().Get(peerID, key)
	if err != nil {
		return "", fmt.Errorf("error getting protocol version:%w", err)
	}
	version, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("error converting interface to string")
	}
	return version, nil
}

func (c *Client) FetchStatus(sFn reqresp.NewStreamFn, ctx context.Context, peer *models.Peer, comp reqresp.Compression) (
	*common.Status, error) {
	resCode := reqresp.ServerErrCode // error by default
	var data *common.Status

	err := methods.StatusRPCv1.RunRequest(ctx, sFn, peer.ID, comp,
		reqresp.RequestSSZInput{Obj: &c.LocalStatus}, 1,
		func() error {
			return nil
		},
		func(chunk reqresp.ChunkedResponseHandler) error {
			resCode = chunk.ResultCode()
			switch resCode {
			case reqresp.ServerErrCode, reqresp.InvalidReqCode:
				msg, err := chunk.ReadErrMsg()
				if err != nil {
					return fmt.Errorf("%s: %w", msg, err)
				}
			case reqresp.SuccessCode:
				var stat common.Status
				if err := chunk.ReadObj(&stat); err != nil {
					return err
				}
				data = &stat
			default:
				return errors.New("unexpected result code")
			}
			return nil
		})
	return data, err
}

func (c *Client) ServeBeaconPing() {
	go func() {
		sCtxFn := func() context.Context {
			reqCtx, _ := context.WithTimeout(c.ctx, RPCTimeout)
			return reqCtx
		}
		comp := new(reqresp.SnappyCompression)
		listenReq := func(ctx context.Context, peerId peer.ID, handler reqresp.ChunkedRequestHandler) {
			log.Info("Handling ping request", log.Ctx{"peer": peerId.String()})
			var ping common.Ping
			err := handler.ReadRequest(&ping)
			if err != nil {
				_ = handler.WriteErrorChunk(reqresp.InvalidReqCode, "could not parse ping request")
				log.Trace("failed to read ping request", log.Ctx{"err:": err, "peer": peerId.String()})
			} else {
				if err := handler.WriteResponseChunk(reqresp.SuccessCode, &ping); err != nil {
					log.Trace("failed to respond to ping request", log.Ctx{"err:": err})
				} else {
					log.Trace("handled ping request")
				}
			}
		}
		m := methods.PingRPCv1
		streamHandler := m.MakeStreamHandler(sCtxFn, comp, listenReq)
		c.Host.SetStreamHandler(m.Protocol, streamHandler)
		log.Info("Started serving ping")
		// wait untill the ctx is down
		<-c.ctx.Done() // TODO: do it better
		log.Info("Stopped serving ping")
	}()
}

func (c *Client) ServeBeaconStatus() {

	go func() {
		sCtxFn := func() context.Context {
			reqCtx, _ := context.WithTimeout(c.ctx, RPCTimeout)
			return reqCtx
		}
		comp := new(reqresp.SnappyCompression)
		listenReq := func(ctx context.Context, peerId peer.ID, handler reqresp.ChunkedRequestHandler) {
			log.Info("Handling status request", log.Ctx{"peer": peerId.String()})
			var reqStatus common.Status
			err := handler.ReadRequest(&reqStatus)
			if err != nil {
				_ = handler.WriteErrorChunk(reqresp.InvalidReqCode, "could not parse status request")
				log.Trace("failed to read status request", log.Ctx{"err:": err, "peer": peerId.String()})
			} else {
				if err := handler.WriteResponseChunk(reqresp.SuccessCode, &c.LocalStatus); err != nil {
					log.Trace("failed to respond to status request", log.Ctx{"err:": err})
				} else {
					// update if possible out status
					c.UpdateStatus(reqStatus)
					log.Trace("handled status request")
				}
			}
		}
		m := methods.StatusRPCv1
		streamHandler := m.MakeStreamHandler(sCtxFn, comp, listenReq)
		c.SetStreamHandler(m.Protocol, streamHandler)
		log.Info("Started serving Beacon Status")
		// wait untill the ctx is down
		<-c.ctx.Done() // TODO: do it better
		log.Info("Stopped serving Beacon Status")
	}()
}

func (c *Client) ServeBeaconMetadata() {
	go func() {
		sCtxFn := func() context.Context {
			reqCtx, _ := context.WithTimeout(c.ctx, RPCTimeout)
			return reqCtx
		}
		comp := new(reqresp.SnappyCompression)
		listenReq := func(ctx context.Context, peerId peer.ID, handler reqresp.ChunkedRequestHandler) {
			log.Info("Handling metadata request", log.Ctx{"peer": peerId.String()})
			var reqMetadata common.MetaData
			err := handler.ReadRequest(&reqMetadata)
			if err != nil {
				_ = handler.WriteErrorChunk(reqresp.InvalidReqCode, "could not parse status request")
				log.Trace("failed to read metadata request", log.Ctx{"err:": err, "peer": peerId.String()})
			} else {
				if err := handler.WriteResponseChunk(reqresp.SuccessCode, &c.LocalMetadata); err != nil {
					log.Trace("failed to respond to metadata request", log.Ctx{"err:": err})
				} else {
					log.Trace("handled metadata request")
				}
			}
		}
		m := methods.MetaDataRPCv2
		streamHandler := m.MakeStreamHandler(sCtxFn, comp, listenReq)
		c.SetStreamHandler(m.Protocol, streamHandler)
		log.Info("Started serving Beacon Metadata")
		// wait untill the ctx is down
		<-c.ctx.Done() // TODO: do it better
		log.Info("Stopped serving Beacon Metadata")
	}()
}

func (c *Client) UpdateStatus(newStatus common.Status) {
	// check if the new one is newer than ours
	if newStatus.HeadSlot > c.LocalStatus.HeadSlot {
		c.LocalStatus = newStatus
	}
}
