package blocktree

import (
	"math/big"
	"math/rand"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/database"
)

func createTestBlockTree(header *types.Header, depth int, db database.Database) *BlockTree {
	bt := NewBlockTreeFromGenesis(header, db)
	previousHash := header.Hash()

	// branch tree randomly
	type testBranch struct {
		hash  Hash
		depth *big.Int
	}

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
		bt.AddBlock(block)
		previousHash = hash

		isBranch := r.Intn(2)
		if isBranch == 1 {
			branches = append(branches, testBranch{
				hash:  hash,
				depth: bt.getNode(hash).depth,
			})
		}
	}

	// create tree branches3
	for _, branch := range branches {
		for i := int(branch.depth.Uint64()); i <= depth; i++ {
			block := &types.Block{
				Header: &types.Header{
					ParentHash: previousHash,
					Number:     big.NewInt(int64(i)),
				},
				Body: &types.Body{},
			}

			hash := block.Header.Hash()
			bt.AddBlock(block)
			previousHash = hash
		}
	}

	return bt
}

func TestStoreBlockTree(t *testing.T) {
	db := database.NewMemDatabase()

	zeroHash, err := common.HexToHash("0x00")
	if err != nil {
		t.Fatal(err)
	}

	header := &types.Header{
		ParentHash: zeroHash,
		Number:     big.NewInt(0),
	}

	bt := createTestBlockTree(header, 100, db)

	err = bt.Store()
	if err != nil {
		t.Fatal(err)
	}

	resBt := NewBlockTreeFromGenesis(header, db)
	err = resBt.Load()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(bt, resBt) {
		t.Fatalf("Fail: got %v expected %v", resBt, bt)
	}
}
