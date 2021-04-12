package cmd

import (
	"encoding/json"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/dgraph-io/ristretto/z"
	"github.com/stretchr/testify/require"
)

func TestConvertJSON(t *testing.T) {
	config := `{
	  "mutations": "strict",
	  "badger": {
	    "compression": "zstd:1",
	    "numgoroutines": 5
	  },
	  "limit": {
		"query_edge": 1000000
	  },
      "raft": {
	    "idx": 2,
	    "learner": true
	  },
	  "security": {
	    "whitelist": "127.0.0.1,0.0.0.0"
	  }
	}`

	var converted map[string]string
	err := json.NewDecoder(convertJSON(config)).Decode(&converted)
	require.NoError(t, err)

	require.Equal(t, "strict", converted["mutations"])

	badger := z.NewSuperFlag(converted["badger"])
	require.Equal(t, "zstd:1", badger.GetString("compression"))
	require.Equal(t, int64(5), badger.GetInt64("numgoroutines"))

	limit := z.NewSuperFlag(converted["limit"])
	require.Equal(t, int64(1000000), limit.GetInt64("query-edge"))

	raft := z.NewSuperFlag(converted["raft"])
	require.Equal(t, int64(2), raft.GetInt64("idx"))
	require.Equal(t, true, raft.GetBool("learner"))

	security := z.NewSuperFlag(converted["security"])
	require.Equal(t, "127.0.0.1,0.0.0.0", security.GetString("whitelist"))
}

func TestConvertYAML(t *testing.T) {
	hier := `
      mutations: strict
      badger:
        compression: zstd:1
        goroutines: 5
      raft:
        idx: 2
        learner: true
      security:
        whitelist: "127.0.0.1,0.0.0.0"`

	conv, err := ioutil.ReadAll(convertYAML(hier))
	if err != nil {
		t.Fatal("error reading from convertYAML")
	}
	unchanged, err := ioutil.ReadAll(convertYAML(string(conv)))
	if err != nil {
		t.Fatal("error reading from convertYAML")
	}
	if string(unchanged) != string(conv) {
		t.Fatal("convertYAML mutating already flattened string")
	}
	if !strings.Contains(string(conv), "compression=zstd:1; goroutines=5;") ||
		!strings.Contains(string(conv), "idx=2; learner=true;") ||
		!strings.Contains(string(conv), "whitelist=127.0.0.1,0.0.0.0") {
		t.Fatal("convertYAML not converting properly")
	}
}
