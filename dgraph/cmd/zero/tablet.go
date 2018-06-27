/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package zero

import (
	"fmt"
	"sort"
	"time"

	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
	humanize "github.com/dustin/go-humanize"
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
				x.Println(err)
			}
		}
	}
}

func (s *Server) movePredicate(predicate string, srcGroup, dstGroup uint32) error {
	// Typically move predicate is run only on leader. But, in this case, an external HTTP request
	// can also trigger a predicate move. We could render them invalid here by checking if this node
	// is actually the leader. But, I have noticed no side effects with allowing them to run, even
	// if this node is a follower node.
	tab := s.ServingTablet(predicate)
	x.AssertTruef(tab != nil, "Tablet to be moved: [%v] should not be nil", predicate)
	x.Printf("Going to move predicate: [%v], size: [%v] from group %d to %d\n", predicate,
		humanize.Bytes(uint64(tab.Space)), srcGroup, dstGroup)

	ctx, cancel := context.WithTimeout(context.Background(), predicateMoveTimeout)
	done := make(chan struct{}, 1)

	go func(done chan struct{}, cancel context.CancelFunc) {
		select {
		case <-s.leaderChangeChannel():
			// Cancel predicate moves when you step down as leader.
			if !s.Node.AmLeader() {
				cancel()
				break
			}

			x.Printf("Sleeping before we run recovery for tablet move")
			// We might have initiated predicate move on some other node, give it some
			// time to get cancelled. On cancellation the other node would set the predicate
			// to write mode again and we need to be sure that it doesn't happen after we
			// decide to move the predicate and set it to read mode.
			time.Sleep(time.Minute)
			// Check if any predicates were stuck in read mode. We don't need to do it
			// periodically because we revert back the predicate to write state in case
			// of any error unless a node crashes or is shutdown.
			s.runRecovery()
		case <-done:
			cancel()
		}
	}(done, cancel)

	err := s.moveTablet(ctx, predicate, srcGroup, dstGroup)
	done <- struct{}{}
	if err != nil {
		return x.Errorf("Error while trying to move predicate %v from %d to %d: %v", predicate,
			srcGroup, dstGroup, err)
	}
	x.Printf("Predicate move done for: [%v] from group %d to %d\n", predicate, srcGroup, dstGroup)
	return nil
}

func (s *Server) runRecovery() {
	s.RLock()
	defer s.RUnlock()
	if s.state == nil {
		return
	}
	var proposals []*intern.ZeroProposal
	for _, group := range s.state.Groups {
		for _, tab := range group.Tablets {
			if tab.ReadOnly {
				p := &intern.ZeroProposal{}
				p.Tablet = &intern.Tablet{
					GroupId:   tab.GroupId,
					Predicate: tab.Predicate,
					Space:     tab.Space,
					Force:     true,
				}
				proposals = append(proposals, p)
			}
		}
	}

	errCh := make(chan error)
	for _, pr := range proposals {
		go func(pr *intern.ZeroProposal) {
			errCh <- s.Node.proposeAndWait(context.Background(), pr)
		}(pr)
	}

	for range proposals {
		// We Don't care about these errors
		// Ideally shouldn't error out.
		if err := <-errCh; err != nil {
			x.Printf("Error while applying proposal in update stream %v\n", err)
		}
	}
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

	x.Printf("\n\nGroups sorted by size: %+v\n\n", groups)
	for lastGroup := numGroups - 1; lastGroup > 0; lastGroup-- {
		srcGroup = groups[lastGroup].gid
		dstGroup = groups[0].gid
		size_diff := groups[lastGroup].size - groups[0].size
		x.Printf("size_diff %v\n", size_diff)
		// Don't move a node unless you receive atleast one update regarding tablet size.
		// Tablet size would have come up with leader update.
		if !s.hasLeader(dstGroup) {
			return
		}
		// We move the predicate only if the difference between size of both machines is
		// atleast 10% of src group.
		if float64(size_diff) < 0.1*float64(groups[0].size) {
			continue
		}

		// Try to find a predicate which we can move.
		size := int64(0)
		group := s.state.Groups[srcGroup]
		for _, tab := range group.Tablets {
			// Finds a tablet as big a possible such that on moving it dstGroup's size is
			// less than or equal to srcGroup.
			if tab.Space <= size_diff/2 && tab.Space > size {
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

func (s *Server) moveTablet(ctx context.Context, predicate string, srcGroup uint32,
	dstGroup uint32) error {
	err := s.movePredicateHelper(ctx, predicate, srcGroup, dstGroup)
	if err == nil {
		// If no error, then return immediately.
		return nil
	}
	x.Printf("Got error during move: %v", err)
	if !s.Node.AmLeader() {
		s.runRecovery()
		return err
	}

	stab := s.ServingTablet(predicate)
	x.AssertTrue(stab != nil)
	p := &intern.ZeroProposal{}
	p.Tablet = &intern.Tablet{
		GroupId:   srcGroup,
		Predicate: predicate,
		Space:     stab.Space,
		Force:     true,
	}
	if nerr := s.Node.proposeAndWait(context.Background(), p); nerr != nil {
		x.Printf("Error while reverting group %d to RW: %+v\n", srcGroup, nerr)
		return nerr
	}
	return err
}

func (s *Server) movePredicateHelper(ctx context.Context, predicate string, srcGroup uint32,
	dstGroup uint32) error {
	n := s.Node
	stab := s.ServingTablet(predicate)
	x.AssertTrue(stab != nil)
	// Propose that predicate in read only
	p := &intern.ZeroProposal{}
	p.Tablet = &intern.Tablet{
		GroupId:   srcGroup,
		Predicate: predicate,
		Space:     stab.Space,
		ReadOnly:  true,
		Force:     true,
	}
	if err := n.proposeAndWait(ctx, p); err != nil {
		return err
	}
	pl := s.Leader(srcGroup)
	if pl == nil {
		return x.Errorf("No healthy connection found to leader of group %d", srcGroup)
	}

	c := intern.NewWorkerClient(pl.Get())
	in := &intern.MovePredicatePayload{
		Predicate:     predicate,
		State:         s.membershipState(),
		SourceGroupId: srcGroup,
		DestGroupId:   dstGroup,
	}
	if _, err := c.MovePredicate(ctx, in); err != nil {
		return fmt.Errorf("While calling MovePredicate: %+v\n", err)
	}

	// Propose that predicate is served by dstGroup in RW.
	p.Tablet = &intern.Tablet{
		GroupId:   dstGroup,
		Predicate: predicate,
		Space:     stab.Space,
		Force:     true,
	}
	if err := n.proposeAndWait(ctx, p); err != nil {
		return err
	}
	// TODO: Probably make it R in dstGroup and send state to srcGroup and only after
	// it proposes make it RW in dstGroup. That way we won't have stale reads from srcGroup
	// for sure.
	return nil
}
