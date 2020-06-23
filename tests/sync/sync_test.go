package sync

import (
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/tests/utils"
	"github.com/stretchr/testify/require"
)

var framework utils.Framework

type testRPCCall struct {
	nodeIdx int
	method  string
	params  string
	delay   time.Duration
}

type checkDBCall struct {
	call1idx int
	call2idx int
	field    string
}

var tests = []testRPCCall{
	{nodeIdx: 0, method: "chain_getHeader", params: "[]", delay: 0},
	{nodeIdx: 1, method: "chain_getHeader", params: "[]", delay: 0},
	{nodeIdx: 2, method: "chain_getHeader", params: "[]", delay: 0},
	{nodeIdx: 0, method: "chain_getHeader", params: "[]", delay: time.Second * 10},
	{nodeIdx: 1, method: "chain_getHeader", params: "[]", delay: 0},
	{nodeIdx: 2, method: "chain_getHeader", params: "[]", delay: 0},
}

var checks = []checkDBCall{
	{call1idx: 0, call2idx: 1, field: "parentHash"},
	{call1idx: 0, call2idx: 2, field: "parentHash"},
	{call1idx: 3, call2idx: 4, field: "parentHash"},
	{call1idx: 3, call2idx: 5, field: "parentHash"},
}

func TestMain(m *testing.M) {
	if utils.MODE != "sync" {
		_, _ = fmt.Fprintln(os.Stdout, "Going to skip stress test")
		return
	}
	fw, err := utils.InitFramework(3)
	if err != nil {
		log.Fatal(fmt.Errorf("error initializing test framework"))
	}
	framework = *fw
	// Start all tests
	code := m.Run()
	os.Exit(code)
}

// this starts nodes and runs RPC calls (which loads db)
func TestCalls(t *testing.T) {
	err := framework.StartNodes(t)
	require.Len(t, err, 0)
	for _, call := range tests {
		time.Sleep(call.delay)
		_, err := framework.CallRPC(call.nodeIdx, call.method, call.params)
		require.NoError(t, err)
	}

	framework.PrintDB()

	// test check
	for _, check := range checks {
		res := framework.CheckEqual(check.call1idx, check.call2idx, check.field)
		require.True(t, res)
	}

	err = framework.KillNodes(t)
	require.Len(t, err, 0)
}
