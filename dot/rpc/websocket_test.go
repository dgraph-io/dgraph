package rpc

import (
	"flag"
	"log"
	"net/url"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/core"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

var addr = flag.String("addr", "localhost:8546", "http service address")
var testCalls = []struct {
	call     []byte
	expected []byte
}{
	{[]byte(`{"jsonrpc":"2.0","method":"system_name","params":[],"id":1}`), []byte(`{"id":1,"jsonrpc":"2.0","result":"gossamer v0.0"}` + "\n")},                                                       // working request
	{[]byte(`{"jsonrpc":"2.0","method":"unknown","params":[],"id":1}`), []byte(`{"error":{"code":-32000,"data":null,"message":"rpc error method unknown not found"},"id":1,"jsonrpc":"2.0"}` + "\n")}, // unknown method
	{[]byte{}, []byte(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid request"},"id":null}` + "\n")},                                                                                      // empty request
	{[]byte(`{"jsonrpc":"2.0","method":"chain_subscribeNewHeads","params":[],"id":1}`), []byte(`{"jsonrpc":"2.0","result":1,"id":1}` + "\n")},
}

func TestNewWebSocketServer(t *testing.T) {
	coreAPI := core.NewTestService(t, nil)
	cfg := &HTTPServerConfig{
		Modules: []string{"system", "chain"},
		RPCPort: 8545,
		WSPort:  8546,
		RPCAPI:  NewService(),
		CoreAPI: coreAPI,
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
