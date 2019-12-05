package grandpa

import (
	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/crypto"
)

//nolint:structcheck
type Voter struct {
	key     crypto.Keypair //nolint:unused
	voterId uint64         //nolint:unused
}

// nolint:structcheck
type State struct {
	voters  []Voter //nolint:unused
	counter uint64  //nolint:unused
	round   uint64  //nolint:unused
}

// nolint:structcheck
type Vote struct {
	hash   common.Hash //nolint:unused
	number uint64      //nolint:unused
}

// nolint:structcheck
type VoteMessage struct {
	round   uint64   //nolint:unused
	counter uint64   //nolint:unused
	pubkey  [32]byte //nolint:unused // ed25519 public key
	stage   byte     //nolint:unused  // 0 for pre-vote, 1 for pre-commit
}

// nolint:structcheck
type Justification struct {
	vote      Vote     //nolint:unused
	signature []byte   //nolint:unused
	pubkey    [32]byte //nolint:unused
}

// nolint:structcheck
type FinalizationMessage struct {
	round         uint64        //nolint:unused
	vote          Vote          //nolint:unused
	justification Justification //nolint:unused
}
