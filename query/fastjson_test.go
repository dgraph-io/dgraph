package query

import (
	"bytes"
	"math"
	"testing"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/roaring/roaring64"
	"github.com/stretchr/testify/require"
)

func subgraphWithSingleResultAndSingleValue(val *pb.TaskValue) *SubGraph {
	r := roaring64.New()
	r.Add(1)
	return &SubGraph{
		Params:    params{Alias: "query"},
		SrcUIDs:   &pb.List{Uids: []uint64{1}},
		DestMap:   r,
		uidMatrix: []*pb.List{&pb.List{Uids: []uint64{1}}},
		Children: []*SubGraph{
			&SubGraph{
				Attr:      "val",
				SrcUIDs:   &pb.List{Uids: []uint64{1}},
				uidMatrix: []*pb.List{&pb.List{}},
				valueMatrix: []*pb.ValueList{
					// UID 1
					&pb.ValueList{
						Values: []*pb.TaskValue{val},
					},
				},
			},
		},
	}
}

func assertJSON(t *testing.T, expected string, sg *SubGraph) {
	buf, err := ToJson(&Latency{}, []*SubGraph{sg})
	require.Nil(t, err)
	require.Equal(t, expected, string(buf))
}

func TestSubgraphToFastJSON(t *testing.T) {
	t.Run("With a string result", func(t *testing.T) {
		sg := subgraphWithSingleResultAndSingleValue(task.FromString("ABC"))
		assertJSON(t, `{"query":[{"val":"ABC"}]}`, sg)
	})

	t.Run("With an integer result", func(t *testing.T) {
		sg := subgraphWithSingleResultAndSingleValue(task.FromInt(42))
		assertJSON(t, `{"query":[{"val":42}]}`, sg)
	})

	t.Run("With a valid float result", func(t *testing.T) {
		sg := subgraphWithSingleResultAndSingleValue(task.FromFloat(42.0))
		assertJSON(t, `{"query":[{"val":42.000000}]}`, sg)
	})

	t.Run("With invalid floating points", func(t *testing.T) {
		assertJSON(t, `{"query":[]}`, subgraphWithSingleResultAndSingleValue(task.FromFloat(math.NaN())))
		assertJSON(t, `{"query":[]}`, subgraphWithSingleResultAndSingleValue(task.FromFloat(math.Inf(1))))
	})
}

func TestEncode(t *testing.T) {
	enc := newEncoder()

	t.Run("with uid list predicate", func(t *testing.T) {
		root := enc.newNode(0)
		friendNode1 := enc.newNode(enc.idForAttr("friend"))
		enc.AddValue(friendNode1, enc.idForAttr("name"),
			types.Val{Tid: types.StringID, Value: "alice"})
		friendNode2 := enc.newNode(enc.idForAttr("friend"))
		enc.AddValue(friendNode2, enc.idForAttr("name"),
			types.Val{Tid: types.StringID, Value: "bob"})

		enc.AddListChild(root, friendNode1)
		enc.AddListChild(root, friendNode2)

		buf := new(bytes.Buffer)
		require.NoError(t, enc.encode(root, buf))
		testutil.CompareJSON(t, `
		{
			"friend":[
				{
					"name":"alice"
				},
				{
					"name":"bob"
				}
			]
		}
		`, buf.String())
	})

	t.Run("with value list predicate", func(t *testing.T) {
		root := enc.newNode(0)
		enc.AddValue(root, enc.idForAttr("name"),
			types.Val{Tid: types.StringID, Value: "alice"})
		enc.AddValue(root, enc.idForAttr("name"),
			types.Val{Tid: types.StringID, Value: "bob"})

		buf := new(bytes.Buffer)
		require.NoError(t, enc.encode(root, buf))
		testutil.CompareJSON(t, `
		{
			"name":[
				"alice",
				"bob"
			]
		}
		`, buf.String())
	})

	t.Run("with uid predicate", func(t *testing.T) {
		root := enc.newNode(0)

		person := enc.newNode(enc.idForAttr("person"))
		enc.AddValue(person, enc.idForAttr("name"), types.Val{Tid: types.StringID, Value: "alice"})
		enc.AddValue(person, enc.idForAttr("age"), types.Val{Tid: types.IntID, Value: 25})

		enc.AddListChild(root, person)

		buf := new(bytes.Buffer)
		require.NoError(t, enc.encode(root, buf))
		testutil.CompareJSON(t, `
		{
			"person":[
				{
					"name":"alice",
					"age":25
				}
			]
		}
		`, buf.String())
	})
}
