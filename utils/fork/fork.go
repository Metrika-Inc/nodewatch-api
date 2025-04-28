package fork

import (
	"context"
	clock "eth2-crawler/utils/clock"
	"eth2-crawler/utils/config"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/protolambda/zrnt/eth2/beacon/common"
)

type ForkChoice struct {
	lock       sync.RWMutex
	Clock      *clock.Clock
	ForkConfig *config.Fork

	// Internal epoch state
	currentEpoch      int64
	currentForkDigest common.ForkDigest
}

func NewForkChoice(ctx context.Context, clock *clock.Clock, forkConfig *config.Fork) *ForkChoice {
	f := &ForkChoice{
		Clock:      clock,
		ForkConfig: forkConfig,
	}

	f.Start(ctx)

	return f
}

func (f *ForkChoice) Fork() common.ForkDigest {
	f.lock.RLock()
	defer f.lock.RUnlock()

	return f.currentForkDigest
}

func (f *ForkChoice) Start(ctx context.Context) {
	currentSlot := f.Clock.CurrentSlot(time.Now().Unix())
	f.currentEpoch = f.Clock.EpochForSlot(currentSlot)
	f.currentForkDigest = *f.getForkForEpoch(f.currentEpoch)

	go f.monitorForkChange(ctx)
}

func (f *ForkChoice) monitorForkChange(ctx context.Context) {

	go func() {
		epochCh := f.Clock.TickEpochs(ctx)
		for epoch := range epochCh {
			f.currentEpoch = epoch
			digest := *f.getForkForEpoch(epoch)

			fmt.Printf("current digest: %s, new: %s \n", f.currentForkDigest.String(), digest.String())
			if digest != f.currentForkDigest {
				f.lock.Lock()
				log.Info("switching fork digest", "fork", digest.String(), "epoch", epoch)

				f.currentForkDigest = digest
				f.lock.Unlock()
			}
		}
	}()
}

func (f *ForkChoice) getForkForEpoch(epoch int64) *common.ForkDigest {
	digest := new(common.ForkDigest)
	name := ""

	if f.currentEpoch >= f.ForkConfig.Electra.ForkEpoch && f.ForkConfig.Electra.Supported {
		name = "electra"
		digest.UnmarshalText([]byte(f.ForkConfig.Electra.ForkDigest))
	} else if f.currentEpoch >= f.ForkConfig.Deneb.ForkEpoch && f.ForkConfig.Deneb.Supported {
		name = "denab"
		digest.UnmarshalText([]byte(f.ForkConfig.Deneb.ForkDigest))
	} else if f.currentEpoch >= f.ForkConfig.Capella.ForkEpoch && f.ForkConfig.Capella.Supported {
		name = "capella"
		digest.UnmarshalText([]byte(f.ForkConfig.Capella.ForkDigest))
	} else if f.currentEpoch >= f.ForkConfig.Bellatrix.ForkEpoch && f.ForkConfig.Bellatrix.Supported {
		name = "bellatrix"
		digest.UnmarshalText([]byte(f.ForkConfig.Bellatrix.ForkDigest))
	} else if f.currentEpoch >= f.ForkConfig.Altair.ForkEpoch && f.ForkConfig.Altair.Supported {
		name = "altair"
		digest.UnmarshalText([]byte(f.ForkConfig.Altair.ForkDigest))
	}
	fmt.Printf("Epoch: %d, Name: %s, Digest: %s\n", epoch, name, digest.String())

	return digest
}
