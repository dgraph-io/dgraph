package posting

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/vector_indexer/index"
)

type FloatVector []float64
type UintSlice []uint64

func getInsertLayer(maxNeighbors, maxLevels int) int {
	// multFactor is a multiplicative factor used to normalize the distribution
	var level int
	randFloat := rand.Float64()
	for i := 0; i < maxLevels; i++ {
		// calculate level based on section 3.1 here
		if randFloat < math.Pow(1.0/float64(maxNeighbors), float64(maxLevels-1-i)) {
			level = i
			break
		}
	}
	return level
}

func searchBadgerLayer(cache *LocalCache, txn *Txn, isInsert bool, pred string, level int, entry uint64, query []float64, expectedNeighbors int, filter index.SearchFilter) ([]minBadgerHeapElement, map[minBadgerHeapElement]bool, error) {
	var nns []minBadgerHeapElement            // track nearest neighbors to return
	var visited map[minBadgerHeapElement]bool // track all visited elements to lock on insert mutation
	entryKey := x.DataKey(pred, entry)
	var pl *List
	var err error
	// insert and query have two diff methods of accessing cache so use isInsert flag to keep track
	if isInsert {
		pl, err = txn.Get(entryKey)
	} else {
		pl, err = cache.Get(entryKey)
	}
	data, err := pl.Value(txn.StartTs)
	if err != nil {
		return []minBadgerHeapElement{}, map[minBadgerHeapElement]bool{}, err
	}
	var startVec *[]float64
	unmarshalErr := json.Unmarshal(data.Value.([]byte), startVec)
	if unmarshalErr != nil {
		return []minBadgerHeapElement{}, map[minBadgerHeapElement]bool{}, unmarshalErr
	}
	// startVec := BytesAsFloatArray(data) //from vfloat type code not pushed yet
	bestDist, err := euclidianDistance(*startVec, query)
	if err != nil {
		return []minBadgerHeapElement{}, map[minBadgerHeapElement]bool{}, err
	}
	best := minBadgerHeapElement{
		value: bestDist,
		index: entry,
	}
	nns = []minBadgerHeapElement{best}
	//create set using map to append to on future visited nodes
	visited = map[minBadgerHeapElement]bool{best: true}
	candidateHeap := *buildBadgerHeapByInit([]minBadgerHeapElement{best})

	for candidateHeap.Len() != 0 {
		currCandidate := candidateHeap.Pop().(minBadgerHeapElement)
		if nns[len(nns)-1].value < currCandidate.value {
			break
		}

		candidateKey := x.DataKey(pred+"_vector_"+fmt.Sprint(level), currCandidate.index)

		var pl *List
		var err error
		if isInsert {
			pl, err = txn.Get(candidateKey)
		} else {
			pl, err = cache.Get(candidateKey)
		}
		data, err := pl.Value(txn.StartTs)
		if err != nil {
			return []minBadgerHeapElement{}, map[minBadgerHeapElement]bool{}, err
		}
		eVecs := [][]float64{}
		var edges *[]uint64
		unmarshalErr := json.Unmarshal(data.Value.([]byte), edges)
		if unmarshalErr != nil {
			return []minBadgerHeapElement{}, map[minBadgerHeapElement]bool{}, unmarshalErr
		}
		for _, edge := range *edges {
			key := x.DataKey(pred, edge)
			var pl *List
			var err error
			if isInsert {
				pl, err = txn.Get(key)
			} else {
				pl, err = cache.Get(key)
			}
			data, err := pl.Value(txn.StartTs)
			if err != nil {
				return []minBadgerHeapElement{}, map[minBadgerHeapElement]bool{}, err
			}
			var eVec *[]float64
			unmarshalErr := json.Unmarshal(data.Value.([]byte), eVec)
			if unmarshalErr != nil {
				return []minBadgerHeapElement{}, map[minBadgerHeapElement]bool{}, unmarshalErr
			}
			eVecs = append(eVecs, *eVec)
		}
		for i := range *edges {
			currDist, err := euclidianDistance(eVecs[i], query) // iterate over candidate's neighbors distances to get best ones
			if err != nil {
				return []minBadgerHeapElement{}, map[minBadgerHeapElement]bool{}, err
			}
			edgesDeref := *edges
			currElement := initBadgerHeapElement(currDist, edgesDeref[i])
			_, nodeExists := visited[*currElement]
			if !nodeExists {
				visited[*currElement] = true

				// push only better vectors that pass filter into candidate heap and add to nearest neighbors
				if filter(query, eVecs[i], edgesDeref[i]) && (currDist < nns[len(nns)-1].value || len(nns) < expectedNeighbors) {
					candidateHeap.Push(*currElement)
					nns = insortBadgerHeapAscending(nns, *currElement)
					if len(nns) > expectedNeighbors {
						nns = nns[:len(nns)-1]
					}
				}

			}
		}
	}

	return nns, visited, nil
}

func newBadgerEdgeKeyValueEntry(ctx context.Context, plist *List, txn *Txn, pred string, level int, uuid uint64, edges []byte) error {
	edge := &pb.DirectedEdge{
		Entity:    uuid,
		Attr:      pred + "_vector_" + fmt.Sprint(level),
		Value:     edges,
		ValueType: pb.Posting_ValType(0),
		Op:        pb.DirectedEdge_SET,
	}
	if err := plist.addMutation(ctx, txn, edge); err != nil {
		return err
	}
	return nil
}

func entryUuidInsert(ctx context.Context, plist *List, txn *Txn, pred string, entryUuid []byte) error {
	edge := &pb.DirectedEdge{
		Entity:    1,
		Attr:      pred + "_vector_entry",
		Value:     entryUuid,
		ValueType: pb.Posting_ValType(7),
		Op:        pb.DirectedEdge_SET,
	}
	if err := plist.addMutation(ctx, txn, edge); err != nil {
		return err
	}
	return nil
}

func InsertToBadger(ctx context.Context, txn *Txn, inUuid uint64, pred string, maxLevels int, maxNeighbors int, efConstruction int) (map[minBadgerHeapElement]bool, error) {
	// str := pred + "_vector_" + fmt.Sprint(maxLevels-1)
	// duplicateCheckKey := x.DataKey(str, inUuid)
	// dup, dupErr := txn.Get(duplicateCheckKey)
	// if dupErr == nil && dup == nil {
	// 	return map[minBadgerHeapElement]bool{}, nil
	// }

	entryKey := x.DataKey(pred+"_vector_entry", 1) // 0-profile_vector_entry
	pl, err := txn.Get(entryKey)
	if err != nil {
		fmt.Println("oh ok")
	}
	data, valErr := pl.Value(txn.StartTs)
	// if valErr != nil {
	// 	return map[minBadgerHeapElement]bool{}, valErr
	// }
	if valErr != nil {
		// if valErr.Error() == "No value found" {
		// no entries in vector index yet b/c no entry exists, so put in all levels
		for i := 0; i < maxLevels; i++ {
			key := x.DataKey(pred+"_vector_"+fmt.Sprint(i), inUuid)
			pl, err := txn.Get(key)
			if err != nil {
				return map[minBadgerHeapElement]bool{}, err
			}
			newBadgerEdgeKeyValueEntry(ctx, pl, txn, pred, i, inUuid, []byte{})
		}
		inUuidByte := make([]byte, 8)
		binary.BigEndian.PutUint64(inUuidByte, inUuid)  // convert inUuid to bytes
		entryUuidInsert(ctx, pl, txn, pred, inUuidByte) // add inUuid as entry for this structure from now on
		return map[minBadgerHeapElement]bool{}, nil
	}
	entry := binary.BigEndian.Uint64(data.Value.([]byte)) // convert entry Uuid returned from Get to uint64
	if entry == inUuid {                                  // something interesting is you physically cannot add duplicate nodes, it'll just overwrite w the same info
		// only situation where you can add duplicate nodes is if youre mutation adds the same node as entry
		return map[minBadgerHeapElement]bool{}, nil
	}

	inLevel := getInsertLayer(maxNeighbors, maxLevels) // calculate layer to insert node at (randomized every time)
	var startVecs []minBadgerHeapElement               // vectors used to calc where to start up until inLevel
	var nns []minBadgerHeapElement                     // nearest neighbors to return after
	var visited map[minBadgerHeapElement]bool          // visited nodes to use later to lock them? TODO
	var inVec *[]float64
	var layerErr error
	for level := 0; level < maxLevels; level++ {
		// perform insertion for layers [level, max_level) only, when level < inLevel just find better start
		if level < inLevel {
			key := x.DataKey(pred, inUuid)
			pl, err := txn.Get(key)
			data, err := pl.Value(txn.StartTs) // Reading this pl doesnt work...?
			if err != nil {
				return map[minBadgerHeapElement]bool{}, err
			}
			unmarshalErr := json.Unmarshal(data.Value.([]byte), inVec) // retrieve vector from inUuid save as inVec
			if unmarshalErr != nil {
				return map[minBadgerHeapElement]bool{}, unmarshalErr
			}
			startVecs, visited, err = searchBadgerLayer(nil, txn, true, pred, level, entry, *inVec, 1, index.AcceptAll)
			if err != nil {
				return map[minBadgerHeapElement]bool{}, err
			}
			entry = startVecs[0].index // update entry to best uuid from current level
		} else {
			nns, visited, layerErr = searchBadgerLayer(nil, txn, true, pred, level, entry, *inVec, efConstruction, index.AcceptAll)
			if layerErr != nil {
				return map[minBadgerHeapElement]bool{}, layerErr
			}
			outboundEdges := []uint64{}
			for i := 0; i < min(len(nns), maxNeighbors); i++ { // iterate over nns at this layer to approx find what to add as edges
				// key := pred + "_vector_" + fmt.Sprint(level) + "_" + fmt.Sprint(nns[i].index)
				key := x.DataKey(pred+"_vector_"+fmt.Sprint(level), nns[i].index)
				pl, err := txn.Get(key)
				data, err := pl.Value(txn.StartTs)
				if err != nil {
					return map[minBadgerHeapElement]bool{}, err
				}
				var nnEdges *[]uint64
				unmarshalErr := json.Unmarshal(data.Value.([]byte), nnEdges) // edges of nearest neighbor
				if unmarshalErr != nil {
					return map[minBadgerHeapElement]bool{}, unmarshalErr
				}
				nnEdgesDeref := *nnEdges
				if len(nnEdgesDeref) < maxNeighbors { // check if # of nn edges are up to maximum. If < max, append, otherwise replace last edge w in Uuid
					nnEdgesDeref = append(nnEdgesDeref, inUuid)
				} else {
					nnEdgesDeref[len(nnEdgesDeref)-1] = inUuid
				}
				inboundEdgesBytes, marshalErr := json.Marshal(nnEdgesDeref)
				if marshalErr != nil {
					return map[minBadgerHeapElement]bool{}, marshalErr
				}
				newBadgerEdgeKeyValueEntry(ctx, pl, txn, pred, level, nns[i].index, inboundEdgesBytes) // This is only supposed to update existing key value pair, is this okay?
				outboundEdges = append(outboundEdges, nns[i].index)                                    // add nn to outboundEdges
			}
			outboundEdgesBytes, marshalErr := json.Marshal(outboundEdges)
			if marshalErr != nil {
				return map[minBadgerHeapElement]bool{}, marshalErr
			}
			key := x.DataKey(pred+"_vector_"+fmt.Sprint(level), inUuid)
			pl, err := txn.Get(key)
			if err != nil {
				return map[minBadgerHeapElement]bool{}, err
			}
			newBadgerEdgeKeyValueEntry(ctx, pl, txn, pred, level, inUuid, outboundEdgesBytes) // add outboundEdges as value to inUuid key
		}
	}
	return visited, nil
}

func Search(cache *LocalCache, query []float64, maxLevels int, pred string, entry uint64, maxResults int, efSearch int, filter index.SearchFilter) ([]uint64, error) {

	for level := 0; level < maxLevels; level++ {
		currBestNns, _, err := searchBadgerLayer(cache, nil, false, pred, level, entry, query, efSearch, index.AcceptAll)
		if err != nil {
			return []uint64{}, err
		}
		entry = currBestNns[0].index
	}
	nn_vals, _, err := searchBadgerLayer(cache, nil, false, pred, maxLevels-1, entry, query, efSearch, filter)
	if err != nil {
		return []uint64{}, err
	}
	var nn_uids []uint64
	for _, nn_val := range nn_vals {
		nn_uids = append(nn_uids, nn_val.index)
	}
	return nn_uids, nil
}

//need Plist for each mutation maxLevel # of posting list
// uid: 0x1 attr: 0-profile_vector_1 plist1
// uid: 0x2 attr: 0-profile_vector_1 plist2

// uid: 0x1 attr: 0-profile_vector_2 plist3
// uid: 0x2 attr: 0-profile_vector_2 plist4
