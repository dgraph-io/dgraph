package blocktree

import (
	"math/big"
	"math/rand"
	"reflect"
	"testing"

	database "github.com/ChainSafe/chaindb"
	"github.com/ChainSafe/gossamer/dot/types"
)

type testBranch struct {
	hash  Hash
	depth *big.Int
}

func createTestBlockTree(header *types.Header, depth int, db database.Database) (*BlockTree, []testBranch) {
	bt := NewBlockTreeFromGenesis(header, db)
	previousHash := header.Hash()

	// branch tree randomly
	branches := []testBranch{}
	r := *rand.New(rand.NewSource(rand.Int63()))

	// create base tree
	for i := 1; i <= depth; i++ {
		block := &types.Block{
			Header: &types.Header{
				ParentHash: previousHash,
				Number:     big.NewInt(int64(i)),
			},
			Body: &types.Body{},
		}

		hash := block.Header.Hash()
		bt.AddBlock(block, 0)
		previousHash = hash

		isBranch := r.Intn(2)
		if isBranch == 1 {
			branches = append(branches, testBranch{
				hash:  hash,
				depth: bt.getNode(hash).depth,
			})
		}
	}

	// create tree branches
	for _, branch := range branches {
		previousHash = branch.hash

		for i := int(branch.depth.Uint64()); i <= depth; i++ {
			block := &types.Block{
				Header: &types.Header{
					ParentHash: previousHash,
					Number:     big.NewInt(int64(i)),
					Digest:     [][]byte{{byte(rand.Intn(256))}},
				},
				Body: &types.Body{},
			}

			hash := block.Header.Hash()
			bt.AddBlock(block, 0)
			previousHash = hash
		}
	}

	return bt, branches
}

func TestStoreBlockTree(t *testing.T) {
	db := database.NewMemDatabase()
	bt, _ := createTestBlockTree(testHeader, 10, db)

	err := bt.Store()
	if err != nil {
		t.Fatal(err)
	}

	resBt := NewBlockTreeFromGenesis(testHeader, db)
	err = resBt.Load()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(bt.head, resBt.head) {
		t.Fatalf("Fail: got %v expected %v", resBt, bt)
	}

	btLeafMap := bt.leaves.toMap()
	resLeafMap := bt.leaves.toMap()
	if !reflect.DeepEqual(btLeafMap, resLeafMap) {
		t.Fatalf("Fail: got %v expected %v", btLeafMap, resLeafMap)
	}
}
