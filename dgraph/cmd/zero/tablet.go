/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package zero

import (
	"context"
	"fmt"
	"sort"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	attribute "go.opentelemetry.io/otel/attribute"
	trace "go.opentelemetry.io/otel/trace"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
)

const (
	// predicateMoveTimeout is the minimum time a predicate move may run before Zero cancels it.
	// moveTimeout extends it for tablets too large to transfer within it at minMoveRate.
	predicateMoveTimeout = 2 * time.Hour

	// minMoveRate is the assumed floor throughput of a predicate move. It converts tablet size
	// into time when computing the move timeout, so that a large tablet is not cancelled by a
	// wall-clock limit it could never meet. The rate is deliberately conservative: the receiving
	// group applies the whole stream through serial Raft proposals, so sustained rates are low.
	minMoveRate = 256 << 10 // bytes per second

	// moveFailureMinElapsed separates real move failures from quick validation ones: only
	// attempts that ran at least this long feed the rebalancer's backoff.
	moveFailureMinElapsed = time.Minute

	// moveBackoffBase and moveBackoffMax bound the cooldown during which the automatic
	// rebalancer skips a tablet whose last move failed. The cooldown doubles per consecutive
	// failure, and is never shorter than the failed attempt itself.
	moveBackoffBase = time.Hour
	moveBackoffMax  = 24 * time.Hour
)

// moveTimeout returns how long a move of tab may run before Zero cancels it: at least base,
// extended for large tablets assuming they transfer no faster than minMoveRate.
func moveTimeout(base time.Duration, tab *pb.Tablet) time.Duration {
	size := max(tab.OnDiskBytes, tab.UncompressedBytes)
	return max(base, time.Duration(size/minMoveRate)*time.Second)
}

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

// TODO: Have a event log for everything.
func (s *Server) rebalanceTablets() {
	ticker := time.Tick(opts.rebalanceInterval)
	for range ticker {
		predicate, srcGroup, dstGroup := s.chooseTablet()
		if len(predicate) == 0 {
			continue
		}
		if err := s.movePredicate(predicate, srcGroup, dstGroup); err != nil {
			glog.Errorln(err)
		}
	}
}

// MoveTablet can be used to move a tablet to a specific group.
// It takes in tablet and destination group as argument.
// It returns a *pb.Status to be used by the `/moveTablet` HTTP handler in Zero.
func (s *Server) MoveTablet(ctx context.Context, req *pb.MoveTabletRequest) (*pb.Status, error) {
	if !s.Node.AmLeader() {
		return &pb.Status{Code: 1, Msg: x.Error}, errNotLeader
	}

	knownGroups := s.KnownGroups()
	var isKnown bool
	for _, grp := range knownGroups {
		if grp == req.DstGroup {
			isKnown = true
			break
		}
	}
	if !isKnown {
		return &pb.Status{Code: 1, Msg: x.ErrorInvalidRequest},
			fmt.Errorf("group: [%d] is not a known group", req.DstGroup)
	}

	tablet := x.NamespaceAttr(req.Namespace, req.Tablet)
	tab := s.ServingTablet(tablet)
	if tab == nil {
		return &pb.Status{Code: 1, Msg: x.ErrorInvalidRequest},
			fmt.Errorf("namespace: %d. No tablet found for: %s", req.Namespace, req.Tablet)
	}

	srcGroup := tab.GroupId
	if srcGroup == req.DstGroup {
		return &pb.Status{Code: 1, Msg: x.ErrorInvalidRequest},
			fmt.Errorf("namespace: %d. Tablet: [%s] is already being served by group: [%d]",
				req.Namespace, req.Tablet, srcGroup)
	}

	if err := s.movePredicate(tablet, srcGroup, req.DstGroup); err != nil {
		glog.Errorf("namespace: %d. While moving predicate %s from %d -> %d. Error: %v",
			req.Namespace, req.Tablet, srcGroup, req.DstGroup, err)
		return &pb.Status{Code: 1, Msg: x.Error}, err
	}

	return &pb.Status{Code: 0, Msg: fmt.Sprintf("namespace: %d. "+
		"Predicate: [%s] moved from group [%d] to [%d]", req.Namespace, req.Tablet, srcGroup,
		req.DstGroup)}, nil
}

// movePredicate is the main entry point for move predicate logic. This Zero must remain the leader
// for the entire duration of predicate move. If this Zero stops being the leader, the final
// proposal of reassigning the tablet to the destination would fail automatically.
func (s *Server) movePredicate(predicate string, srcGroup, dstGroup uint32) (err error) {
	s.moveOngoing <- struct{}{}
	defer func() {
		<-s.moveOngoing
	}()

	// Ensure that reserved predicates cannot be moved.
	if x.IsReservedPredicate(predicate) {
		return errors.Errorf("Unable to move reserved predicate %s", predicate)
	}
	tab := s.ServingTablet(predicate)
	if tab == nil {
		return errors.Errorf("Tablet to be moved: [%v] is not being served", predicate)
	}

	// Feed the outcome of this attempt back to the rebalancer, so it stops re-picking a tablet
	// whose moves keep failing.
	start := time.Now()
	defer func() {
		s.recordMoveResult(predicate, time.Since(start), err)
	}()

	timeout := moveTimeout(predicateMoveTimeout, tab)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	span := trace.SpanFromContext(ctx)
	defer span.End()

	// Ensure that I'm connected to the rest of the Zero group, and am the leader.
	if _, err := s.latestMembershipState(ctx); err != nil {
		return errors.Wrapf(err, "unable to reach quorum")
	}
	if !s.Node.AmLeader() {
		return errors.Errorf("I am not the Zero leader")
	}
	msg := fmt.Sprintf("Going to move predicate: [%v], size: [ondisk: %v, uncompressed: %v]"+
		" from group %d to %d, timeout: %v\n", predicate, humanize.IBytes(uint64(tab.OnDiskBytes)),
		humanize.IBytes(uint64(tab.UncompressedBytes)), srcGroup, dstGroup, timeout)
	glog.Info(msg)
	span.SetAttributes(attribute.String("tablet", predicate))
	span.SetStatus(1, msg)

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
	span.AddEvent(fmt.Sprintf("Move Predicate payload: %+v", in))
	glog.Infof("Starting move: %+v", in)
	if _, err := wc.MovePredicate(ctx, in); err != nil {
		return errors.Wrapf(err, "while calling MovePredicate")
	}

	p := &pb.ZeroProposal{}
	p.Tablet = &pb.Tablet{
		GroupId:           dstGroup,
		Predicate:         predicate,
		OnDiskBytes:       tab.OnDiskBytes,
		UncompressedBytes: tab.UncompressedBytes,
		Force:             true,
		MoveTs:            in.TxnTs,
	}
	msg = fmt.Sprintf("Move at Alpha done. Now proposing: %+v", p)
	span.AddEvent(fmt.Sprintf("Zero proposal: %+v", p))
	glog.Info(msg)
	if err := s.Node.proposeAndWait(ctx, p); err != nil {
		return errors.Wrapf(err, "while proposing tablet reassignment. Proposal: %+v", p)
	}
	msg = fmt.Sprintf("Predicate move done for: [%v] from group %d to %d\n",
		predicate, srcGroup, dstGroup)
	span.AddEvent(msg)
	glog.Info(msg)

	// Now that the move has happened, we can delete the predicate from the source group. But before
	// doing that, we should ensure the source group understands that the predicate is now being
	// served by the destination group. For that, we pass in the expected checksum for the source
	// group. Only once the source group membership checksum matches, would the source group delete
	// the predicate. This ensures that it does not service any transaction after deletion of data.
	checksums := s.groupChecksums()
	in.ExpectedChecksum = checksums[in.SourceGid]
	in.DestGid = 0 // Indicates deletion of predicate in the source group.
	if _, err := wc.MovePredicate(ctx, in); err != nil {
		msg = fmt.Sprintf("While deleting predicate [%v] in group %d. Error: %v",
			in.Predicate, in.SourceGid, err)
		span.AddEvent(msg)
		glog.Warningf(msg)
	} else {
		msg = fmt.Sprintf("Deleted predicate %v in group %d", in.Predicate, in.SourceGid)
		span.AddEvent(msg)
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
			space += tab.OnDiskBytes
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
		// atleast 10% of dst group.
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
			// Skip tablets whose recent moves failed; retrying right away would repeat hours of
			// streaming and commit-blocking for the same outcome.
			if s.skipMove(tab.Predicate) {
				continue
			}

			// Finds a tablet as big a possible such that on moving it dstGroup's size is
			// less than or equal to srcGroup.
			if tab.OnDiskBytes <= sizeDiff/2 && tab.OnDiskBytes > size {
				predicate = tab.Predicate
				size = tab.OnDiskBytes
			}
		}
		if len(predicate) > 0 {
			return
		}
	}
	return
}

// tabletBackoff records the automatic rebalancer's backoff state for one tablet.
type tabletBackoff struct {
	failures int
	until    time.Time
}

// moveCooldown returns how long the automatic rebalancer should skip a tablet after its
// failures-th consecutive move failure, where elapsed is how long the failed attempt ran.
func moveCooldown(failures int, elapsed time.Duration) time.Duration {
	cooldown := moveBackoffMax
	if shift := failures - 1; shift >= 0 && shift < 6 {
		cooldown = min(moveBackoffBase<<shift, moveBackoffMax)
	}
	return max(cooldown, elapsed)
}

// recordMoveResult updates the rebalancer's backoff state for pred after a move attempt that
// ran for elapsed. A successful move clears the state. A failed attempt that did real work
// (ran at least moveFailureMinElapsed) pushes the next automatic attempt out by moveCooldown.
// Only chooseTablet consults this state, so manual moves via MoveTablet are never held back.
func (s *Server) recordMoveResult(pred string, elapsed time.Duration, err error) {
	if err == nil {
		s.moveBackoff.Delete(pred)
		return
	}
	if elapsed < moveFailureMinElapsed {
		return
	}
	failures := 1
	if prev, ok := s.moveBackoff.Load(pred); ok {
		failures = prev.(tabletBackoff).failures + 1
	}
	cooldown := moveCooldown(failures, elapsed)
	s.moveBackoff.Store(pred, tabletBackoff{failures: failures, until: time.Now().Add(cooldown)})
	glog.Infof("Move of tablet [%v] failed after %v (failure #%d). Skipping automatic"+
		" rebalancing of this tablet for %v", pred, elapsed.Round(time.Second), failures, cooldown)
}

// skipMove reports whether the automatic rebalancer should currently leave pred alone because
// its last move attempt failed.
func (s *Server) skipMove(pred string) bool {
	entry, ok := s.moveBackoff.Load(pred)
	return ok && time.Now().Before(entry.(tabletBackoff).until)
}
