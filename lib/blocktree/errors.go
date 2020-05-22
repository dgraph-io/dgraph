package blocktree

import (
	"errors"
)

// ErrParentNotFound is returned if the parent hash does not exist in the blocktree
var ErrParentNotFound = errors.New("cannot find parent block in blocktree")

// ErrBlockExists is returned if attempting to re-add a block
var ErrBlockExists = errors.New("cannot add block to blocktree that already exists")

// ErrStartNodeNotFound is returned if the start of a subchain does not exist
var ErrStartNodeNotFound = errors.New("start node does not exist")

// ErrEndNodeNotFound is returned if the end of a subchain does not exist
var ErrEndNodeNotFound = errors.New("end node does not exist")

// ErrNilDatabase is returned in the database is nil
var ErrNilDatabase = errors.New("blocktree database is nil")

// ErrNilDescendant is returned if calling subchain with a nil node
var ErrNilDescendant = errors.New("descendant node is nil")

// ErrDescendantNotFound is returned if a descendant in a subchain cannot be found
var ErrDescendantNotFound = errors.New("could not find descendant node")

// ErrNodeNotFound is returned if a node with given hash doesn't exist
var ErrNodeNotFound = errors.New("could not find node")
