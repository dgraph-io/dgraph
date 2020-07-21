package sync

import (
	"time"

	log "github.com/ChainSafe/log15"
)

type benchmarker struct {
	logger          log.Logger
	start           time.Time
	startBlock      uint64
	blocksPerSecond []float64
	syncing         bool
}

func newBenchmarker(logger log.Logger) *benchmarker {
	return &benchmarker{
		logger:          logger.New("module", "benchmarker"),
		blocksPerSecond: []float64{},
	}
}

func (b *benchmarker) begin(block uint64) {
	if !b.syncing {
		b.start = time.Now()
		b.startBlock = block
		b.syncing = true
	}
}

func (b *benchmarker) end(block uint64) {
	if !b.syncing {
		return
	}

	duration := time.Since(b.start)
	blocks := block - b.startBlock
	if blocks == 0 {
		blocks = 1
	}
	bps := float64(blocks) / duration.Seconds()
	b.blocksPerSecond = append(b.blocksPerSecond, bps)

	b.logger.Info("added sync time", "start", b.startBlock, "end", block, "duration", duration.Seconds(), "recent blocks/second", bps, "avg blocks/second", b.average())
	b.syncing = false
}

func (b *benchmarker) average() float64 {
	sum := float64(0)
	for _, bps := range b.blocksPerSecond {
		sum += bps
	}
	return sum / float64(len(b.blocksPerSecond))
}
