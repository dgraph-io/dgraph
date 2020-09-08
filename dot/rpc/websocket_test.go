package rpc

import (
	"flag"
	"log"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/core"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/system"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

var addr = flag.String("addr", "localhost:8546", "http service address")
var testCalls = []struct {
	call     []byte
	expected []byte
}{
	{[]byte(`{"jsonrpc":"2.0","method":"system_name","params":[],"id":1}`), []byte(`{"id":1,"jsonrpc":"2.0","result":"gossamer"}` + "\n")},                                                            // working request
	{[]byte(`{"jsonrpc":"2.0","method":"unknown","params":[],"id":1}`), []byte(`{"error":{"code":-32000,"data":null,"message":"rpc error method unknown not found"},"id":1,"jsonrpc":"2.0"}` + "\n")}, // unknown method
	{[]byte{}, []byte(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid request"},"id":0}` + "\n")},                                                                                         // empty request
	{[]byte(`{"jsonrpc":"2.0","method":"chain_subscribeNewHeads","params":[],"id":3}`), []byte(`{"jsonrpc":"2.0","result":1,"id":3}` + "\n")},
	{[]byte(`{"jsonrpc":"2.0","method":"state_subscribeStorage","params":[],"id":4}`), []byte(`{"jsonrpc":"2.0","result":2,"id":4}` + "\n")},
}

func TestHTTPServer_ServeHTTP(t *testing.T) {
	coreAPI := core.NewTestService(t, nil)
	si := &types.SystemInfo{
		SystemName: "gossamer",
	}
	sysAPI := system.NewService(si)
	bAPI := new(MockBlockAPI)
	sAPI := new(MockStorageAPI)
	cfg := &HTTPServerConfig{
		Modules:    []string{"system", "chain"},
		RPCPort:    8545,
		WSPort:     8546,
		WSEnabled:  true,
		RPCAPI:     NewService(),
		CoreAPI:    coreAPI,
		SystemAPI:  sysAPI,
		BlockAPI:   bAPI,
		StorageAPI: sAPI,
	}

	s := NewHTTPServer(cfg)
	err := s.Start()
	require.Nil(t, err)

	time.Sleep(time.Second) // give server a second to start

	u := url.URL{Scheme: "ws", Host: *addr, Path: "/"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	for _, item := range testCalls {
		err = c.WriteMessage(websocket.TextMessage, item.call)
		require.Nil(t, err)

		_, message, err := c.ReadMessage()
		require.Nil(t, err)
		require.Equal(t, item.expected, message)
	}
}

type MockBlockAPI struct {
}

func (m *MockBlockAPI) GetHeader(hash common.Hash) (*types.Header, error) {
	return nil, nil
}
func (m *MockBlockAPI) BestBlockHash() common.Hash {
	return common.Hash{}
}
func (m *MockBlockAPI) GetBlockByHash(hash common.Hash) (*types.Block, error) {
	return nil, nil
}
func (m *MockBlockAPI) GetBlockHash(blockNumber *big.Int) (*common.Hash, error) {
	return nil, nil
}
func (m *MockBlockAPI) GetFinalizedHash(uint64, uint64) (common.Hash, error) {
	return common.Hash{}, nil
}
func (m *MockBlockAPI) RegisterImportedChannel(ch chan<- *types.Block) (byte, error) {
	return 0, nil
}
func (m *MockBlockAPI) UnregisterImportedChannel(id byte) {
}

type MockStorageAPI struct{}

func (m *MockStorageAPI) GetStorage(_ *common.Hash, key []byte) ([]byte, error) {
	return nil, nil
}
func (m *MockStorageAPI) Entries(_ *common.Hash) (map[string][]byte, error) {
	return nil, nil
}
func (m *MockStorageAPI) GetStorageByBlockHash(_ common.Hash, key []byte) ([]byte, error) {
	return nil, nil
}
func (m *MockStorageAPI) RegisterStorageChangeChannel(ch chan<- *state.KeyValue) (byte, error) {
	return 0, nil
}
func (m *MockStorageAPI) UnregisterStorageChangeChannel(id byte) {

}
