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

package zero

import (
	"fmt"
	"sort"
	"time"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	humanize "github.com/dustin/go-humanize"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	otrace "go.opencensus.io/trace"
	"golang.org/x/net/context"
)

const (
	predicateMoveTimeout = 20 * time.Minute
)

/*
Steps to move predicate p from g1 to g2.
Design change:
• If you’re not the leader, don’t talk to zero.
• Let the leader send you updates via proposals.

Move:
• Dgraph zero would decide that G1 should not serve P, G2 should serve it.
• Zero would propose that G1 is read-only for predicate P. This would propagate to the cluster.

• Zero would tell G1 to move P to G2 (Endpoint: Zero → G1)

This would trigger G1 to get latest state. Wait for it.
• G1 would propose this state to it’s followers.
• G1 after proposing would do a call to G2, and start streaming.
• Before G2 starts accepting, it should delete any current keys for P.
• It should tell Zero whether it succeeded or failed. (Endpoint: G1 → Zero)

• Zero would then propose that G2 is serving P (or G1 is, if fail above) P would RW.
• G1 gets this, G2 gets this.
• Both propagate this to their followers.

*/

//  TODO: Have a event log for everything.
func (s *Server) rebalanceTablets() {
	ticker := time.NewTicker(opts.rebalanceInterval)
	for {
		select {
		case <-ticker.C:
			predicate, srcGroup, dstGroup := s.chooseTablet()
			if len(predicate) == 0 {
				break
			}
			if err := s.movePredicate(predicate, srcGroup, dstGroup); err != nil {
				glog.Errorln(err)
			}
		}
	}
}

// movePredicate is the main entry point for move predicate logic. This Zero must remain the leader
// for the entire duration of predicate move. If this Zero stops being the leader, the final
// proposal of reassigning the tablet to the destination would fail automatically.
func (s *Server) movePredicate(predicate string, srcGroup, dstGroup uint32) error {
	s.moveOngoing <- struct{}{}
	defer func() {
		<-s.moveOngoing
	}()

	ctx, cancel := context.WithTimeout(context.Background(), predicateMoveTimeout)
	defer cancel()

	ctx, span := otrace.StartSpan(ctx, "Zero.MovePredicate")
	defer span.End()

	// Ensure that reserved predicates cannot be moved.
	if x.IsReservedPredicate(predicate) {
		return errors.Errorf("Unable to move reserved predicate %s", predicate)
	}

	// Ensure that I'm connected to the rest of the Zero group, and am the leader.
	if _, err := s.latestMembershipState(ctx); err != nil {
		return errors.Wrapf(err, "unable to reach quorum")
	}
	if !s.Node.AmLeader() {
		return errors.Errorf("I am not the Zero leader")
	}
	tab := s.ServingTablet(predicate)
	if tab == nil {
		return errors.Errorf("Tablet to be moved: [%v] is not being served", predicate)
	}
	msg := fmt.Sprintf("Going to move predicate: [%v], size: [%v] from group %d to %d\n", predicate,
		humanize.Bytes(uint64(tab.Space)), srcGroup, dstGroup)
	glog.Info(msg)
	span.Annotate([]otrace.Attribute{otrace.StringAttribute("tablet", predicate)}, msg)

	// Block all commits on this predicate. Keep them blocked until we return from this function.
	unblock := s.blockTablet(predicate)
	defer unblock()

	// Get a new timestamp, beyond which we are sure that no new txns would be committed for this
	// predicate. Source Alpha leader must reach this timestamp before streaming the data.
	ids, err := s.Timestamps(ctx, &pb.Num{Val: 1})
	if err != nil || ids.StartId == 0 {
		return errors.Wrapf(err, "while leasing txn timestamp. Id: %+v", ids)
	}

	// Get connection to leader of source group.
	pl := s.Leader(srcGroup)
	if pl == nil {
		return errors.Errorf("No healthy connection found to leader of group %d", srcGroup)
	}
	wc := pb.NewWorkerClient(pl.Get())
	in := &pb.MovePredicatePayload{
		Predicate: predicate,
		SourceGid: srcGroup,
		DestGid:   dstGroup,
		TxnTs:     ids.StartId,
	}
	span.Annotatef(nil, "Starting move: %+v", in)
	glog.Infof("Starting move: %+v", in)
	if _, err := wc.MovePredicate(ctx, in); err != nil {
		return fmt.Errorf("While calling MovePredicate: %+v\n", err)
	}

	p := &pb.ZeroProposal{}
	p.Tablet = &pb.Tablet{
		GroupId:   dstGroup,
		Predicate: predicate,
		Space:     tab.Space,
		Force:     true,
	}
	msg = fmt.Sprintf("Move at Alpha done. Now proposing: %+v", p)
	span.Annotate(nil, msg)
	glog.Info(msg)
	if err := s.Node.proposeAndWait(ctx, p); err != nil {
		return errors.Wrapf(err, "while proposing tablet reassignment. Proposal: %+v", p)
	}
	msg = fmt.Sprintf("Predicate move done for: [%v] from group %d to %d\n",
		predicate, srcGroup, dstGroup)
	glog.Info(msg)
	span.Annotate(nil, msg)

	// Now that the move has happened, we can delete the predicate from the source group.
	in.DestGid = 0 // Indicates deletion of predicate in the source group.
	if _, err := wc.MovePredicate(ctx, in); err != nil {
		msg = fmt.Sprintf("While deleting predicate [%v] in group %d. Error: %v",
			in.Predicate, in.SourceGid, err)
		span.Annotate(nil, msg)
		glog.Warningf(msg)
	} else {
		msg = fmt.Sprintf("Deleted predicate %v in group %d", in.Predicate, in.SourceGid)
		span.Annotate(nil, msg)
		glog.V(1).Infof(msg)
	}
	return nil
}

func (s *Server) chooseTablet() (predicate string, srcGroup uint32, dstGroup uint32) {
	s.RLock()
	defer s.RUnlock()
	if s.state == nil {
		return
	}
	numGroups := len(s.state.Groups)
	if !s.Node.AmLeader() || numGroups <= 1 {
		return
	}

	// Sort all groups by their sizes.
	type kv struct {
		gid  uint32
		size int64 // in bytes
	}
	var groups []kv
	for k, v := range s.state.Groups {
		space := int64(0)
		for _, tab := range v.Tablets {
			space += tab.Space
		}
		groups = append(groups, kv{k, space})
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].size < groups[j].size
	})

	glog.Infof("\n\nGroups sorted by size: %+v\n\n", groups)
	for lastGroup := numGroups - 1; lastGroup > 0; lastGroup-- {
		srcGroup = groups[lastGroup].gid
		dstGroup = groups[0].gid
		sizeDiff := groups[lastGroup].size - groups[0].size
		glog.Infof("size_diff %v\n", sizeDiff)
		// Don't move a node unless you receive atleast one update regarding tablet size.
		// Tablet size would have come up with leader update.
		if !s.hasLeader(dstGroup) {
			return
		}
		// We move the predicate only if the difference between size of both machines is
		// atleast 10% of src group.
		if float64(sizeDiff) < 0.1*float64(groups[0].size) {
			continue
		}

		// Try to find a predicate which we can move.
		size := int64(0)
		group := s.state.Groups[srcGroup]
		for _, tab := range group.Tablets {
			// Reserved predicates should always be in group 1 so do not re-balance them.
			if x.IsReservedPredicate(tab.Predicate) {
				continue
			}

			// Finds a tablet as big a possible such that on moving it dstGroup's size is
			// less than or equal to srcGroup.
			if tab.Space <= sizeDiff/2 && tab.Space > size {
				predicate = tab.Predicate
				size = tab.Space
			}
		}
		if len(predicate) > 0 {
			return
		}
	}
	return
}
