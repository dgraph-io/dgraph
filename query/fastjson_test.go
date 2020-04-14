package query

import (
	"math"
	"testing"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/task"
	"github.com/stretchr/testify/require"
)

func subgraphWithSingleResultAndSingleValue(val *pb.TaskValue) *SubGraph {
	return &SubGraph{
		Params:    params{Alias: "query"},
		SrcUIDs:   &pb.List{Uids: []uint64{1}},
		DestUIDs:  &pb.List{Uids: []uint64{1}},
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
