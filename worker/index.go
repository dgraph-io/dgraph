/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package worker

import (
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

func (n *node) rebuildOrDelIndex(ctx context.Context, attr string, rebuild bool, startTs uint64) error {
	rv := ctx.Value("raft").(x.RaftValue)
	x.AssertTrue(rv.Group == n.gid)

	if schema.State().IsIndexed(attr) != rebuild {
		return x.Errorf("Predicate %s index mismatch, rebuild %v", attr, rebuild)
	}
	// Remove index edges
	if err := posting.DeleteIndex(attr); err != nil {
		return err
	}
	if rebuild {
		if err := posting.RebuildIndex(ctx, attr, startTs); err != nil {
			return err
		}
	}
	return nil
}

func (n *node) rebuildOrDelRevEdge(ctx context.Context, attr string, rebuild bool, startTs uint64) error {
	rv := ctx.Value("raft").(x.RaftValue)
	x.AssertTrue(rv.Group == n.gid)

	if schema.State().IsReversed(attr) != rebuild {
		return x.Errorf("Predicate %s reverse mismatch, rebuild %v", attr, rebuild)
	}
	if err := posting.DeleteReverseEdges(attr); err != nil {
		return err
	}
	if rebuild {
		// Remove reverse edges
		if err := posting.RebuildReverseEdges(ctx, attr, startTs); err != nil {
			return err
		}
	}
	return nil
}

func (n *node) rebuildOrDelCountIndex(ctx context.Context, attr string, rebuild bool, startTs uint64) error {
	rv := ctx.Value("raft").(x.RaftValue)
	x.AssertTrue(rv.Group == n.gid)

	if err := posting.DeleteCountIndex(attr); err != nil {
		return err
	}
	if rebuild {
		if err := posting.RebuildCountIndex(ctx, attr, startTs); err != nil {
			return err
		}
	}
	return nil
}
