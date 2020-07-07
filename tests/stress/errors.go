package stress

import (
	"errors"
)

var errFinalizedBlockMismatch = errors.New("node finalized head hashes don't match")
var errNoFinalizedBlock = errors.New("did not finalize block for round")
var errNoBlockAtNumber = errors.New("no blocks found for given number")
var errBlocksAtNumberMismatch = errors.New("different blocks found for given number")
var errChainHeadMismatch = errors.New("node chain head hashes don't match")
