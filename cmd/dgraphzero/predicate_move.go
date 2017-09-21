/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"sort"
	"time"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/context"
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

// TODO: Handle failure scenarios, probably cleanup
// 1. Zero can crash after proposing G1 is read only. Need to run recovery on restart.
// 2. Cleanup
// 3. Verify timeout when destiantion node is slow.
// 4. Leader change in group zero, new leader needs to check whether some move was in process.
// 5. Crashed once during testing, don't know why :(
func (s *Server) rebalanceTablets() {
	ticker := time.NewTicker(time.Minute)
	for {
		<-ticker.C
		s.RLock()
		if s.state == nil {
			s.RUnlock()
			return
		}
		numGroups := len(s.state.Groups)
		if !s.Node.AmLeader() || numGroups <= 1 {
			continue
		}

		// Sort all groups by their sizes.
		type kv struct {
			gid  uint32
			size int64
		}
		var ss []kv
		for k, v := range s.state.Groups {
			ss = append(ss, kv{k, v.Size_})
		}
		s.RUnlock()
		sort.Slice(ss, func(i, j int) bool {
			return ss[i].size < ss[j].size
		})

		// TODO: Start moving more at a time, for now we will move 1.
		srcGroup := ss[numGroups-1].gid
		dstGroup := ss[0].gid
		size_diff := ss[numGroups-1].size - ss[0].size
		x.Printf("\n\nGroups sorted by size: %+v, size_diff %v\n\n", ss, size_diff)
		// TODO: Don't move a node unless you receive atleast one update regarding tablet size.
		if size_diff < 50<<20 || ss[0].size == 0 { // Change limit later
			continue
		}

		// Try to find a predicate which we can move.
		predicate := ""
		size := int64(0)
		s.RLock()
		group := s.state.Groups[srcGroup]
		for _, tab := range group.Tablets {
			// Finds a tablet as big a possible such that on moving it dstGroup's size is
			// less than or equal to srcGroup.
			// TODO: Probably don't drop below the mean of all groups.
			if tab.Size_ <= size_diff/2 && tab.Size_ > size {
				predicate = tab.Predicate
				size = tab.Size_
			}
		}
		s.RUnlock()
		if len(predicate) == 0 {
			continue
		}
		x.Printf("Going to move predicate %v from %d to %d\n", predicate, srcGroup, dstGroup)
		if err := s.movePredicate(predicate, srcGroup, dstGroup); err != nil {
			x.Printf("Error while trying to move predicate %v from %d to %d: %v\n",
				predicate, srcGroup, dstGroup, err)
		}
	}
}

func (s *Server) movePredicate(predicate string, srcGroup uint32,
	dstGroup uint32) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*20)
	defer cancel()
	err := s.movePredicateHelper(ctx, predicate, srcGroup, dstGroup)
	if err == nil {
		return nil
	}

	stab := s.ServingTablet(predicate)
	x.AssertTrue(stab != nil)
	p := &protos.ZeroProposal{}
	p.Tablet = &protos.Tablet{
		GroupId:   srcGroup,
		Predicate: predicate,
		Size_:     stab.Size_,
	}
	if err := s.Node.proposeAndWait(context.Background(), p); err != nil {
		x.Printf("Error while reverting group %d to RW", srcGroup)
	}
	return err
}

func (s *Server) movePredicateHelper(ctx context.Context, predicate string, srcGroup uint32,
	dstGroup uint32) error {
	n := s.Node
	stab := s.ServingTablet(predicate)
	x.AssertTrue(stab != nil)
	// Propose that predicate in read only
	p := &protos.ZeroProposal{}
	p.Tablet = &protos.Tablet{
		GroupId:   srcGroup,
		Predicate: predicate,
		Size_:     stab.Size_,
		ReadOnly:  true,
	}
	if err := n.proposeAndWait(ctx, p); err != nil {
		return err
	}
	pl := s.Leader(srcGroup)
	if pl == nil {
		return x.Errorf("No healthy connection found to leader of group %d", srcGroup)
	}

	c := protos.NewWorkerClient(pl.Get())
	in := &protos.MovePredicatePayload{
		Predicate:     predicate,
		State:         s.membershipState(),
		SourceGroupId: srcGroup,
		DestGroupId:   dstGroup,
	}
	if _, err := c.MovePredicate(ctx, in); err != nil {
		return err
	}

	// Propose that predicate is served by dstGroup in RW.
	p.Tablet = &protos.Tablet{
		GroupId:   dstGroup,
		Predicate: predicate,
		Size_:     stab.Size_,
		PrevGroup: srcGroup,
	}
	if err := n.proposeAndWait(ctx, p); err != nil {
		return err
	}
	// TODO: Probably make it R in dstGroup and send state to srcGroup and only after
	// it proposes make it RW in dstGroup. That way we won't have stale reads from srcGroup
	// for sure.
	return nil
}
