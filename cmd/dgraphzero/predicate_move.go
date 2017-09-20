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

Cleanup:
// Could cause race conditons when predicate is being moved. Ensure only either deletePredicate
// is running or predicate streaming.
// But if we are sending from g1 to g2, stream breaks in between then cleanup guy would end up
// cleaning the predicate(Would create tombstones). Group zero would anyhow retry moving later.
// TODO: Decide whether it's better if group zero does the cleanup.
• The iterator for tablet update can do the cleanup, if it’s not serving the predicate.
*/

// TODO: Handle failure scenarios, probably cleanup
// Zero can crash after proposing G1 is read only. Need to run recovery on restart.
// Ensure that we don't end up moving same predicate from g1 to g2 and then g2 to g1.
func (s *Server) movePredicate(predicate string, srcGroup uint32,
	dstGroup uint32) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*20)
	defer cancel()
	err := s.movePredicateHelper(ctx, predicate, srcGroup, dstGroup)
	if err == nil {
		return true, nil
	}

	stab := s.servingTablet(predicate)
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
	return false, err
}

func (s *Server) movePredicateHelper(ctx context.Context, predicate string, srcGroup uint32,
	dstGroup uint32) error {
	n := s.Node
	stab := s.servingTablet(predicate)
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
	}
	if err := n.proposeAndWait(ctx, p); err != nil {
		return err
	}
	// TODO: Probably make it R in dstGroup and send state to srcGroup and only after
	// it proposes make it RW in dstGroup. That way we won't have stale reads from srcGroup
	// for sure.
	return nil
}
