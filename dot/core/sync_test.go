package core

import (
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/common/variadic"
	"github.com/ChainSafe/gossamer/lib/trie"
)

func newTestSyncer(t *testing.T, cfg *SyncerConfig) *Syncer {
	if cfg == nil {
		cfg = &SyncerConfig{}
	}

	cfg.Lock = &sync.Mutex{}
	cfg.ChanLock = &sync.Mutex{}

	stateSrvc := state.NewService("")
	stateSrvc.UseMemDB()

	err := stateSrvc.Initialize(testGenesisHeader, trie.NewEmptyTrie(nil))
	if err != nil {
		t.Fatal(err)
	}

	err = stateSrvc.Start()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.BlockState == nil {
		cfg.BlockState = stateSrvc.Block
	}

	if cfg.BlockNumIn == nil {
		cfg.BlockNumIn = make(chan *big.Int)
	}

	if cfg.RespIn == nil {
		cfg.RespIn = make(chan *network.BlockResponseMessage)
	}

	if cfg.MsgOut == nil {
		cfg.MsgOut = make(chan network.Message)
	}

	syncer, err := NewSyncer(cfg)
	if err != nil {
		t.Fatal(err)
	}

	return syncer
}

func TestWatchForBlocks(t *testing.T) {
	blockNumberIn := make(chan *big.Int)
	msgOut := make(chan network.Message)

	cfg := &SyncerConfig{
		BlockNumIn: blockNumberIn,
		MsgOut:     msgOut,
	}

	syncer := newTestSyncer(t, cfg)
	syncer.Start()

	number := big.NewInt(12)
	blockNumberIn <- number

	var msg network.Message

	select {
	case msg = <-msgOut:
	case <-time.After(testMessageTimeout):
		t.Error("timeout waiting for message")
	}

	req, ok := msg.(*network.BlockRequestMessage)
	if !ok {
		t.Fatal("did not get BlockRequestMessage")
	}

	if req.StartingBlock.Value().(uint64) != 1 {
		t.Fatalf("Fail: got %d expected %d", req.StartingBlock.Value(), 1)
	}

	if syncer.requestStart != 1 {
		t.Fatalf("Fail: got %d expected %d", syncer.requestStart, 1)
	}

	if syncer.highestSeenBlock.Cmp(number) != 0 {
		t.Fatalf("Fail: highestSeenBlock=%d expected %d", syncer.highestSeenBlock, number)
	}
}

func TestWatchForBlocks_NotHighestSeen(t *testing.T) {
	blockNumberIn := make(chan *big.Int)

	cfg := &SyncerConfig{
		BlockNumIn: blockNumberIn,
	}

	syncer := newTestSyncer(t, cfg)
	syncer.Start()

	number := big.NewInt(12)
	blockNumberIn <- number

	if syncer.highestSeenBlock.Cmp(number) != 0 {
		t.Fatalf("Fail: highestSeenBlock=%d expected %d", syncer.highestSeenBlock, number)
	}

	blockNumberIn <- big.NewInt(11)

	if syncer.highestSeenBlock.Cmp(number) != 0 {
		t.Fatalf("Fail: highestSeenBlock=%d expected %d", syncer.highestSeenBlock, number)
	}
}

func TestWatchForBlocks_GreaterThanHighestSeen_NotSynced(t *testing.T) {
	blockNumberIn := make(chan *big.Int)
	msgOut := make(chan network.Message)

	cfg := &SyncerConfig{
		BlockNumIn: blockNumberIn,
		MsgOut:     msgOut,
	}

	syncer := newTestSyncer(t, cfg)
	syncer.Start()

	number := big.NewInt(12)
	blockNumberIn <- number

	if syncer.highestSeenBlock.Cmp(number) != 0 {
		t.Fatalf("Fail: highestSeenBlock=%d expected %d", syncer.highestSeenBlock, number)
	}

	var msg network.Message

	select {
	case msg = <-msgOut:
	case <-time.After(testMessageTimeout):
		t.Error("timeout waiting for message")
	}

	number = big.NewInt(16)
	blockNumberIn <- number

	select {
	case msg = <-msgOut:
	case <-time.After(testMessageTimeout):
		t.Error("timeout waiting for message")
	}

	if syncer.highestSeenBlock.Cmp(number) != 0 {
		t.Fatalf("Fail: highestSeenBlock=%d expected %d", syncer.highestSeenBlock, number)
	}

	req, ok := msg.(*network.BlockRequestMessage)
	if !ok {
		t.Fatal("did not get BlockRequestMessage")
	}

	if req.StartingBlock.Value().(uint64) != 12 {
		t.Fatalf("Fail: got %d expected %d", req.StartingBlock.Value(), 12)
	}
}

func TestWatchForBlocks_GreaterThanHighestSeen_Synced(t *testing.T) {
	blockNumberIn := make(chan *big.Int)
	msgOut := make(chan network.Message)

	cfg := &SyncerConfig{
		BlockNumIn: blockNumberIn,
		MsgOut:     msgOut,
	}

	syncer := newTestSyncer(t, cfg)
	syncer.Start()

	number := big.NewInt(12)
	blockNumberIn <- number

	var msg network.Message

	select {
	case msg = <-msgOut:
	case <-time.After(testMessageTimeout):
		t.Error("timeout waiting for message")
	}

	if syncer.highestSeenBlock.Cmp(number) != 0 {
		t.Fatalf("Fail: highestSeenBlock=%d expected %d", syncer.highestSeenBlock, number)
	}

	// synced to block 12
	syncer.synced = true
	syncer.lock.Unlock()

	number = big.NewInt(16)
	blockNumberIn <- number

	select {
	case msg = <-msgOut:
	case <-time.After(testMessageTimeout):
		t.Error("timeout waiting for message")
	}

	if syncer.highestSeenBlock.Cmp(number) != 0 {
		t.Fatalf("Fail: highestSeenBlock=%d expected %d", syncer.highestSeenBlock, number)
	}

	req, ok := msg.(*network.BlockRequestMessage)
	if !ok {
		t.Fatal("did not get BlockRequestMessage")
	}

	if req.StartingBlock.Value().(uint64) != 13 {
		t.Fatalf("Fail: got %d expected %d", req.StartingBlock.Value(), 13)
	}
}

func TestWatchForResponses(t *testing.T) {
	blockNumberIn := make(chan *big.Int)
	respIn := make(chan *network.BlockResponseMessage)
	msgOut := make(chan network.Message)

	cfg := &SyncerConfig{
		BlockNumIn: blockNumberIn,
		RespIn:     respIn,
		MsgOut:     msgOut,
	}

	syncer := newTestSyncer(t, cfg)
	syncer.Start()

	syncer.highestSeenBlock = big.NewInt(16)

	coreSrv := newTestService(t, nil)
	addTestBlocksToState(t, 16, coreSrv.blockState)

	startNum := 1
	start, err := variadic.NewUint64OrHash(startNum)
	if err != nil {
		t.Fatal(err)
	}

	req := &network.BlockRequestMessage{
		ID:            1,
		RequestedData: 3,
		StartingBlock: start,
	}

	resp, err := coreSrv.createBlockResponse(req)
	if err != nil {
		t.Fatal(err)
	}

	syncer.lock.Lock()
	syncer.synced = false

	respIn <- resp
	time.Sleep(time.Second)

	var msg network.Message

	select {
	case msg = <-msgOut:
	case <-time.After(testMessageTimeout):
		t.Error("timeout waiting for message")
	}

	// msg should contain blocks 1 to 8 (maxResponseSize # of blocks)
	if syncer.synced {
		t.Fatal("Fail: not yet synced")
	}

	req2, ok := msg.(*network.BlockRequestMessage)
	if !ok {
		t.Fatal("did not get BlockRequestMessage")
	}

	if req2.StartingBlock.Value().(uint64) != uint64(startNum+int(maxResponseSize)) {
		t.Fatalf("Fail: got %d expected %d", req2.StartingBlock.Value(), startNum+int(maxResponseSize))
	}

	resp2, err := coreSrv.createBlockResponse(req2)
	if err != nil {
		t.Fatal(err)
	}

	respIn <- resp2
	time.Sleep(time.Second)

	// response should contain blocks 9 to 16, and we should be synced
	if !syncer.synced {
		t.Fatal("Fail: should be synced")
	}
}

func TestWatchForResponses_MissingBlocks(t *testing.T) {
	blockNumberIn := make(chan *big.Int)
	respIn := make(chan *network.BlockResponseMessage)
	msgOut := make(chan network.Message)

	cfg := &SyncerConfig{
		BlockNumIn: blockNumberIn,
		RespIn:     respIn,
		MsgOut:     msgOut,
	}

	syncer := newTestSyncer(t, cfg)
	syncer.Start()

	syncer.highestSeenBlock = big.NewInt(16)

	coreSrv := newTestService(t, nil)
	addTestBlocksToState(t, 16, coreSrv.blockState)

	startNum := 16
	syncer.requestStart = int64(startNum)

	start, err := variadic.NewUint64OrHash(startNum)
	if err != nil {
		t.Fatal(err)
	}

	req := &network.BlockRequestMessage{
		ID:            1,
		RequestedData: 3,
		StartingBlock: start,
	}

	resp, err := coreSrv.createBlockResponse(req)
	if err != nil {
		t.Fatal(err)
	}

	syncer.lock.Lock()
	syncer.synced = false

	respIn <- resp
	time.Sleep(time.Second)

	var msg network.Message

	select {
	case msg = <-msgOut:
	case <-time.After(testMessageTimeout):
		t.Error("timeout waiting for message")
	}

	// msg should contain block 16 (maxResponseSize # of blocks)
	if syncer.synced {
		t.Fatal("Fail: not yet synced")
	}

	req2, ok := msg.(*network.BlockRequestMessage)
	if !ok {
		t.Fatal("did not get BlockRequestMessage")
	}

	if req2.StartingBlock.Value().(uint64) != uint64(startNum-int(maxResponseSize)) {
		t.Fatalf("Fail: got %d expected %d", req2.StartingBlock.Value(), startNum-int(maxResponseSize))
	}

}
