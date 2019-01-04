/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

func (n *node) rebuildOrDelIndex(ctx context.Context, attr string, rebuild bool,
	startTs uint64) error {
	if schema.State().IsIndexed(attr) != rebuild {
		return x.Errorf("Predicate %s index mismatch, rebuild %v", attr, rebuild)
	}
	// Remove index edges
	glog.Infof("Deleting index for %s", attr)
	if err := posting.DeleteIndex(attr); err != nil {
		return err
	}
	if rebuild {
		glog.Infof("Rebuilding index for %s", attr)
		return posting.RebuildIndex(ctx, attr, startTs)
	}
	return nil
}

func (n *node) rebuildOrDelRevEdge(ctx context.Context, attr string, rebuild bool,
	startTs uint64) error {
	if schema.State().IsReversed(attr) != rebuild {
		return x.Errorf("Predicate %s reverse mismatch, rebuild %v", attr, rebuild)
	}
	glog.Infof("Deleting reverse index for %s", attr)
	if err := posting.DeleteReverseEdges(attr); err != nil {
		return err
	}
	if rebuild {
		// Remove reverse edges
		glog.Infof("Rebuilding reverse index for %s", attr)
		return posting.RebuildReverseEdges(ctx, attr, startTs)
	}
	return nil
}

func (n *node) rebuildOrDelCountIndex(ctx context.Context, attr string, rebuild bool,
	startTs uint64) error {
	glog.Infof("Deleting count index for %s", attr)
	if err := posting.DeleteCountIndex(attr); err != nil {
		return err
	}
	if rebuild {
		glog.Infof("Rebuilding count index for %s", attr)
		return posting.RebuildCountIndex(ctx, attr, startTs)
	}
	return nil
}
