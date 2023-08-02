package crawl

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"log"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/host"
)

func startGossipSub(ctx context.Context, host host.Host) (*pubsub.PubSub, error) {
	// Setup the params
	gossipParams := pubsub.DefaultGossipSubParams()

	psOptions := []pubsub.Option{
		pubsub.WithMessageSigning(false),
		pubsub.WithStrictSignatureVerification(false),
		pubsub.WithMessageIdFn(MsgIDFunction),
		pubsub.WithGossipSubParams(gossipParams),
	}
	gs, err := pubsub.NewGossipSub(ctx, host, psOptions...)
	if err != nil {
		log.Panic(err)
	}
	return gs, nil
}

// WithMessageIdFn is an option to customize the way a message ID is computed for a pubsub message
func MsgIDFunction(pmsg *pubsub_pb.Message) string {
	h := sha256.New()
	// never errors, see crypto/sha256 Go doc

	_, _ = h.Write(pmsg.Data)
	id := h.Sum(nil)
	return base64.URLEncoding.EncodeToString(id)
}
