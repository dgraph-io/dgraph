package query

import (
	"bytes"
	"math"
	"testing"

	"github.com/dgraph-io/dgraph/types"
	"github.com/stretchr/testify/require"
)

func assertJSON(t *testing.T, expected string, fj *fastJsonNode) {
	var bufw bytes.Buffer
	err := fj.encode(&bufw)
	require.Nil(t, err)
	require.Equal(t, expected, bufw.String())
}

func TestSubgraphToFastJSON(t *testing.T) {
	// No idea why New is on an object
	var fastJSONFactory *fastJsonNode

	t.Run("serializing an empty fastjson", func(t *testing.T) {
		seedNode := fastJSONFactory.New("_root_")
		assertJSON(t, `{}`, seedNode)
	})

	t.Run("serializing a fastjson with an integer", func(t *testing.T) {
		seedNode := fastJSONFactory.New("_root_")
		seedNode.AddValue("foo", types.Val{Tid: types.IntID, Value: 42})
		assertJSON(t, `{"foo":42}`, seedNode)
	})

	// FIXME: This is returning invalid JSON
	t.Run("serializing a fastjson with an map", func(t *testing.T) {
		seedNode := fastJSONFactory.New("_root_")
		seedNode.AddMapChild("foo", &fastJsonNode{isChild: true}, false)
		assertJSON(t, `{"foo":{}}`, seedNode)
	})

	t.Run("serializing a fastjson with a float", func(t *testing.T) {
		seedNode := fastJSONFactory.New("_root_")
		seedNode.AddValue("foo", types.Val{Tid: types.FloatID, Value: 42.0})
		assertJSON(t, `{"foo":42.000000}`, seedNode)
	})

	t.Run("serializing a fastjson with an invalid float", func(t *testing.T) {
		seedNode := fastJSONFactory.New("_root_")
		seedNode.AddValue("foo", types.Val{Tid: types.FloatID, Value: math.NaN()})
		assertJSON(t, `{}`, seedNode)
	})
}
