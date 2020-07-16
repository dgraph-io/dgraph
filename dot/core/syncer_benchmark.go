package core

import (
	"time"

	log "github.com/ChainSafe/log15"
)

type benchmarker struct {
	logger          log.Logger
	start           time.Time
	startBlock      uint64
	blocksPerSecond []float64
}

func newBenchmarker(logger log.Logger) *benchmarker {
	return &benchmarker{
		logger:          logger.New("module", "benchmarker"),
		blocksPerSecond: []float64{},
	}
}

func (b *benchmarker) begin(block uint64) {
	b.start = time.Now()
	b.startBlock = block
}

func (b *benchmarker) end(block uint64) {
	duration := time.Since(b.start)
	blocks := block - b.startBlock
	bps := float64(blocks) / duration.Seconds()
	b.blocksPerSecond = append(b.blocksPerSecond, bps)

	b.logger.Info("added sync time", "avg blocks/second", b.average())
}

func (b *benchmarker) average() float64 {
	sum := float64(0)
	for _, bps := range b.blocksPerSecond {
		sum += bps
	}
	return sum / float64(len(b.blocksPerSecond))
}
