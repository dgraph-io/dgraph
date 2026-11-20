/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package zero

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	attribute "go.opentelemetry.io/otel/attribute"
	trace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/dgraph-io/badger/v4/y"
	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	"github.com/dgraph-io/dgraph/v25/x"
	"github.com/dgraph-io/ristretto/v2/z"
)

// Oracle stores and manages the transaction state and conflict detection.
type Oracle struct {
	x.SafeMutex
	commits map[uint64]uint64 // startTs -> commitTs
	// TODO: Check if we need LRU.
	keyCommit   *z.Tree // fp(key) -> commitTs. Used to detect conflict.
	maxAssigned uint64  // max transaction assigned by us.

	// All transactions with startTs < startTxnTs return true for hasConflict.
	startTxnTs  uint64
	subscribers map[int]chan pb.OracleDelta
	updates     chan *pb.OracleDelta
	doneUntil   y.WaterMark
}

// Init initializes the oracle.
func (o *Oracle) Init() {
	o.commits = make(map[uint64]uint64)
	// Remove the older btree file, before creating NewTree, as it may contain stale data leading
	// to wrong results.
	o.keyCommit = z.NewTree("oracle")
	o.subscribers = make(map[int]chan pb.OracleDelta)
	o.updates = make(chan *pb.OracleDelta, 100000) // Keeping 1 second worth of updates.
	o.doneUntil.Init(nil)
	go o.sendDeltasToSubscribers()
}

// close releases the memory associated with btree used for keycommit.
func (o *Oracle) close() {
	if err := o.keyCommit.Close(); err != nil {
		glog.Warningf("error while closing tree: %v", err)
	}
}

func (o *Oracle) updateStartTxnTs(ts uint64) {
	o.Lock()
	defer o.Unlock()
	o.startTxnTs = ts
	o.keyCommit.Reset()
}

// TODO: This should be done during proposal application for Txn status.
func (o *Oracle) hasConflict(src *api.TxnContext) bool {
	// This transaction was started before I became leader.
	if src.StartTs < o.startTxnTs {
		return true
	}
	for _, k := range src.Keys {
		ki, err := strconv.ParseUint(k, 36, 64)
		if err != nil {
			glog.Errorf("Got error while parsing conflict key %q: %v\n", k, err)
			continue
		}
		if last := o.keyCommit.Get(ki); last > src.StartTs {
			return true
		}
	}
	return false
}

func (o *Oracle) purgeBelow(minTs uint64) {
	var timer x.Timer
	timer.Start()

	o.Lock()
	defer o.Unlock()

	// Set startTxnTs so that every txn with start ts less than this, would be aborted.
	o.startTxnTs = minTs

	// Dropping would be cheaper if abort/commits map is sharded
	for ts := range o.commits {
		if ts < minTs {
			delete(o.commits, ts)
		}
	}
	timer.Record("commits")

	// There is no transaction running with startTs less than minTs
	// So we can delete everything from rowCommit whose commitTs < minTs
	stats := o.keyCommit.Stats()
	if stats.Occupancy < 50.0 {
		return
	}
	o.keyCommit.DeleteBelow(minTs)
	timer.Record("deleteBelow")
	glog.V(2).Infof("Purged below ts:%d, len(o.commits):%d, keyCommit: [before: %+v, after: %+v].\n",
		minTs, len(o.commits), stats, o.keyCommit.Stats())
	if timer.Total() > time.Second {
		glog.V(2).Infof("Purge %s\n", timer.String())
	}
}

// errStaleStartTs is returned by Oracle.commit when the start timestamp is below startTxnTs. It is
// decided under the same lock as the conflict check, so the late abort path in Server.commit reports
// what the oracle actually saw. Re-reading startTxnTs afterwards could mislabel the abort, because
// purgeBelow and updateStartTxnTs can both move it in between.
var errStaleStartTs = errors.New("transaction start timestamp is stale")

func (o *Oracle) commit(src *api.TxnContext) error {
	o.Lock()
	defer o.Unlock()

	if o.isStaleStartTs(src) {
		return errStaleStartTs
	}
	if o.hasConflict(src) {
		return x.ErrConflict
	}
	// We store src.Keys as string to ensure compatibility with all the various language clients we
	// have. But, really they are just uint64s encoded as strings. We use base 36 during creation of
	// these keys in FillContext in posting/mvcc.go.
	for _, k := range src.Keys {
		ki, err := strconv.ParseUint(k, 36, 64)
		if err != nil {
			glog.Errorf("Got error while parsing conflict key %q: %v\n", k, err)
			continue
		}
		o.keyCommit.Set(ki, src.CommitTs) // CommitTs is handed out before calling this func.
	}
	return nil
}

func (o *Oracle) currentState() *pb.OracleDelta {
	o.AssertRLock()
	resp := &pb.OracleDelta{}
	for start, commit := range o.commits {
		resp.Txns = append(resp.Txns,
			&pb.TxnStatus{StartTs: start, CommitTs: commit})
	}
	resp.MaxAssigned = o.maxAssigned
	return resp
}

func (o *Oracle) newSubscriber() (<-chan pb.OracleDelta, int) {
	o.Lock()
	defer o.Unlock()
	var id int
	for {
		//nolint:gosec // random generator for node id does not require cryptographic precision
		id = rand.Int()
		if _, has := o.subscribers[id]; !has {
			break
		}
	}

	// The channel takes a delta instead of a pointer as the receiver needs to
	// modify it by setting the group checksums. Passing a pointer previously
	// resulted in a race condition.
	ch := make(chan pb.OracleDelta, 1000)
	ch <- *o.currentState() // Queue up the full state as the first entry.
	o.subscribers[id] = ch
	return ch, id
}

func (o *Oracle) removeSubscriber(id int) {
	o.Lock()
	defer o.Unlock()
	delete(o.subscribers, id)
}

// sendDeltasToSubscribers reads updates from the o.updates
// constructs a delta object containing transactions from one or more updates
// and sends the delta object to each subscriber's channel
func (o *Oracle) sendDeltasToSubscribers() {
	delta := &pb.OracleDelta{}
	ticker := time.Tick(time.Second)

	// waitFor calculates the maximum value of delta.MaxAssigned and all the CommitTs of delta.Txns
	waitFor := func() uint64 {
		w := delta.MaxAssigned
		for _, txn := range delta.Txns {
			w = x.Max(w, txn.CommitTs)
		}
		return w
	}

	for {
	get_update:
		var update *pb.OracleDelta
		select {
		case update = <-o.updates:
		case <-ticker:
			wait := waitFor()
			if wait == 0 || o.doneUntil.DoneUntil() < wait {
				goto get_update
			}
			// Send empty update.
			update = &pb.OracleDelta{}
		}
	slurp_loop:
		for {
			delta.MaxAssigned = x.Max(delta.MaxAssigned, update.MaxAssigned)
			delta.Txns = append(delta.Txns, update.Txns...)
			select {
			case update = <-o.updates:
			default:
				break slurp_loop
			}
		}
		// No need to sort the txn updates here. Alpha would sort them before
		// applying.

		// Let's ensure that we have all the commits up until the max here.
		// Otherwise, we'll be sending commit timestamps out of order, which
		// would cause Alphas to drop some of them, during writes to Badger.
		if o.doneUntil.DoneUntil() < waitFor() {
			continue // The for loop doing blocking reads from o.updates.
			// We need at least one entry from the updates channel to pick up a missing update.
			// Don't goto slurp_loop, because it would break from select immediately.
		}

		if glog.V(3) {
			glog.Infof("DoneUntil: %d. Sending delta: %+v\n", o.doneUntil.DoneUntil(), delta)
		}
		o.Lock()
		for id, ch := range o.subscribers {
			select {
			case ch <- *delta:
			default:
				close(ch)
				delete(o.subscribers, id)
			}
		}
		o.Unlock()
		delta = &pb.OracleDelta{}
	}
}

func (o *Oracle) updateCommitStatusHelper(index uint64, src *api.TxnContext) bool {
	o.Lock()
	defer o.Unlock()
	if _, ok := o.commits[src.StartTs]; ok {
		return false
	}
	if src.Aborted {
		o.commits[src.StartTs] = 0
	} else {
		o.commits[src.StartTs] = src.CommitTs
	}
	return true
}

func (o *Oracle) updateCommitStatus(index uint64, src *api.TxnContext) {
	// TODO: We should check if the tablet is in read-only status here.
	if o.updateCommitStatusHelper(index, src) {
		delta := new(pb.OracleDelta)
		delta.Txns = append(delta.Txns, &pb.TxnStatus{
			StartTs:  src.StartTs,
			CommitTs: o.commitTs(src.StartTs),
		})
		o.updates <- delta
	}
}

func (o *Oracle) commitTs(startTs uint64) uint64 {
	o.RLock()
	defer o.RUnlock()
	return o.commits[startTs]
}

func (o *Oracle) storePending(ids *pb.AssignedIds) {
	// Wait to finish up processing everything before start id.
	max := x.Max(ids.EndId, ids.ReadOnly)
	if err := o.doneUntil.WaitForMark(context.Background(), max); err != nil {
		glog.Errorf("Error while waiting for mark: %+v", err)
	}

	// Now send it out to updates.
	o.updates <- &pb.OracleDelta{MaxAssigned: max}

	o.Lock()
	defer o.Unlock()
	o.maxAssigned = x.Max(o.maxAssigned, max)
}

// MaxPending returns the maximum assigned timestamp.
func (o *Oracle) MaxPending() uint64 {
	o.RLock()
	defer o.RUnlock()
	return o.maxAssigned
}

// proposeTxn proposes a txn update, and then updates src to reflect the state
// of the commit after proposal is run.
func (s *Server) proposeTxn(ctx context.Context, src *api.TxnContext) error {
	var zp pb.ZeroProposal
	zp.Txn = &api.TxnContext{
		StartTs:  src.StartTs,
		CommitTs: src.CommitTs,
		Aborted:  src.Aborted,
	}

	// NOTE: It is important that we continue retrying proposeTxn until we succeed. This should
	// happen, irrespective of what the user context timeout might be. We check for it before
	// reaching this stage, but now that we're here, we have to ensure that the commit proposal goes
	// through. Otherwise, we should block here forever. If we don't do this, we'll see txn
	// violations in Jepsen, because we'll send out a MaxAssigned higher than a commit, which would
	// cause newer txns to see older data.

	// If this node stops being the leader, we want this proposal to not be forwarded to the leader,
	// and get aborted.
	if err := s.Node.proposeAndWait(ctx, &zp); err != nil {
		return err
	}

	// This proposal can race with an abort proposed for the same start ts elsewhere: a TryAbort
	// from an Alpha (schema update, drop-predicate, or the idle-transaction reaper), or a client
	// Discard of the same transaction. Both sides propose their respective states and only one
	// can succeed, so check again to see the fate of the transaction here. (A predicate move does
	// not abort pending transactions; it blocks their commits in checkPreds instead.)
	src.CommitTs = s.orc.commitTs(src.StartTs)
	if src.CommitTs == 0 {
		src.Aborted = true
	}
	return nil
}

// Abort-reason codes. When Zero decides to abort a transaction it surfaces the category to the
// client as the prefix of a codes.Aborted gRPC status message, formatted as "<code>: <detail>".
// api.TxnContext (external dgo module) has no field for the reason, so the reason rides on the
// error instead; the status code stays codes.Aborted, so existing abort handling is unaffected.
// Client support for parsing these prefixes into an AbortReason enum is proposed in
// dgraph-io/dgraph4j#290, dgraph-io/pydgraph#307 and dgraph-io/dgo#288; keep the server and
// those parsers in sync.
//
// These three codes are the entire published vocabulary. A cause that cannot be mapped onto one of
// them emits the detail with *no* prefix (abortReasonUncategorized) rather than borrowing a code
// that would imply the wrong remedy. Each of those client parsers maps an absent or unrecognized
// prefix to UNKNOWN, so "no category" is a supported and truthful answer. Withholding
// also keeps a later phase purely additive: naming a new category then only adds precision to
// something that was already UNKNOWN, and never reclassifies a code clients have shipped against.
const (
	// abortReasonUncategorized means the server knows what happened and says so in the detail,
	// but the cause does not correspond to any published category. It is not "the server does not
	// know" — it is "there is no code for this yet".
	abortReasonUncategorized = ""
	abortReasonConflict      = "conflict"
	abortReasonStaleStartTs  = "stale-startts"
	abortReasonPredicateMove = "predicate-move"
)

// Human-readable details, one per cause. Every message a client can receive for a server-decided
// abort is declared here rather than at its call site, so the published surface is a single block:
// adding a new abort means adding a constant next to the existing vocabulary, where the category it
// should pair with is visible. Entries ending in Fmt are format strings and must be given their
// arguments; the others are complete. abortReason frames each one as
// "<category>: Transaction has been aborted. Please retry. <detail>".
//
//	category         detail
//	───────────────  ──────────────────────────────────────────────────────────
//	conflict         abortDetailConflict
//	stale-startts    abortDetailStaleStartTs
//	predicate-move   abortDetailPredicateBlockedFmt, abortDetailGroupMismatchFmt
//	(withheld)       abortDetailMissingGroupIDFmt, abortDetailBadGroupIDFmt,
//	                 abortDetailTabletNilFmt, abortDetailPreAborted, and
//	                 ctx.Err() — the last supplied by the Go runtime, so it is
//	                 not declared here
const (
	// A conflict is the one category the server cannot narrow. Every conflict key reaching Zero is a
	// farm.Fingerprint64(key)^uid produced by GetConflictKey (posting/list.go), so by the time
	// hasConflict matches one, which key produced it is already unrecoverable. Naming the
	// possibilities is therefore the most this category can honestly say, and it is worth saying: a
	// caller who set a single scalar value has no reason to expect that the index and count keys
	// *derived* from it can conflict too.
	//
	// Exactly three kinds of key carry a conflict — data, index and count. Reverse keys carry none:
	// IsReverse is a distinct ByteType, so a reverse key matches no case in GetConflictKey's switch,
	// falls to default, and addConflictKey drops the resulting zero. Do not add "reverse" here.
	//
	// @upsert is named separately because it is a *rule*, not a fourth kind of key. HasUpsert is
	// tested ahead of every IsData/IsIndex case, so on an @upsert predicate the existing data and
	// index keys switch to getKey(key, 0) — the uid drops out and any two transactions writing the
	// same value collide on the shared index key. That is the uniqueness mechanism, and in
	// upsert-heavy ingest it is the most likely cause of this abort, which is why it is worth the
	// extra sentence.
	//
	// abortReason puts abortLegacyMessage in front of this, so the wire message still starts with the
	// sentence that dgraph/cmd/alpha/upsert_test.go, query/query4_test.go and systest/unique_test.go
	// match against the live server.
	abortDetailConflict = "Another transaction committed to one of the same keys. The conflicting " +
		"key cannot be identified: it may be the data key written directly, or an index or count " +
		"key derived from it. On an @upsert predicate the uid is excluded, so any two transactions " +
		"writing the same value conflict"
	// A start timestamp goes stale for two different reasons, so this detail deliberately names
	// both rather than asserting one. Zero raises startTxnTs either when a Zero becomes leader and
	// renews its leases (updateLeases, assign.go) or when it trims its conflict map while applying
	// a snapshot (purgeBelow via applySnapshot, raft.go) — the second is not a leader change at
	// all. Both mean the same thing to a caller: this transaction's start timestamp is older than
	// the oldest timestamp Zero can still validate against, so retry with a fresh transaction.
	abortDetailStaleStartTs = "Its start timestamp is older than the oldest timestamp Zero can " +
		"still validate (Zero leader change, or its conflict map was trimmed at a snapshot)"

	// Reported when proposeTxn comes back with the transaction already aborted while this commit
	// decided nothing itself — i.e. something else proposed an abort for the same start timestamp
	// before this commit was arbitrated. Zero records no cause when that happens (an abort proposal
	// carries only timestamps), so the category is withheld and the detail names the possibilities
	// instead of guessing. None of them is a predicate *move* — a move blocks commits in checkPreds
	// before a commit timestamp is leased, and never aborts pending transactions — so
	// predicate-move would be the wrong label. The known producers are:
	//
	//   - a schema or type update on a predicate the transaction touched, which cancels pending
	//     transactions so the index can be rebuilt (detectPendingTxns, worker/draft.go)
	//   - a drop-predicate (S * * delete) on a predicate the transaction touched (same function)
	//   - the Alpha leader ageing out transactions idle longer than --limit "txn-abort-after"
	//     (abortOldTransactions, worker/draft.go)
	//   - a client Discard of the same transaction racing this commit, which reaches commit()'s own
	//     src.Aborted early return and proposes the abort there
	//
	// The first three reach Zero through TryAbort (blockingAbort, worker/draft.go).
	//
	// Before this was handled, commit() returned nil here and the abort reached the client as a
	// bare dgo.ErrAborted with no reason at all.
	abortDetailPreAborted = "It was already aborted before this commit was decided, for example " +
		"because a schema update or a drop-predicate cancelled pending transactions on a predicate " +
		"it touched, the server aged it out after --limit \"txn-abort-after\" of idleness, or the " +
		"client discarded it concurrently"

	// Details produced by checkPreds. The wording predates the abort-reason work, but commit() used to
	// discard these errors, so clients have never seen them. The predicate argument is the name as
	// the user wrote it (see displayPred), not the namespaced attribute.
	abortDetailMissingGroupIDFmt   = "Unable to find group id in %s"
	abortDetailBadGroupIDFmt       = "unable to parse group id from %s"
	abortDetailTabletNilFmt        = "Tablet for %s is nil"
	abortDetailPredicateBlockedFmt = "Commits on predicate %s are blocked due to predicate move"
	abortDetailGroupMismatchFmt    = "Mutation done in group: %d. Predicate %s assigned to %d"
)

// abortLegacyMessage is the text of every abort from a server without abort reasons, and the text
// of dgo.ErrAborted. abortReason puts it in front of every detail, so each abort message still
// contains it. Clients that recognise aborts by message text rely on that: the dgraph-js-http
// client treats an HTTP error as an abort only when its message contains "abort" and "retry", and
// rethrows anything else as a hard failure. Tests in this repo and user log-scrapers also
// substring-match it. Never change it, and never emit an abort that bypasses abortReason.
const abortLegacyMessage = "Transaction has been aborted. Please retry"

// abortReason builds the wire string the client parses: "<code>: <legacy sentence>. <detail>". An
// empty code drops the "<code>: " prefix, so the client reports no category, but the message still
// starts with the legacy sentence a server without this feature sent for every abort.
func abortReason(code, detail string) string {
	msg := abortLegacyMessage + ". " + detail
	if code == abortReasonUncategorized {
		return msg
	}
	return code + ": " + msg
}

// conflictAbortReason returns the wire reason for an abort decided by hasConflict, distinguishing a
// write-write conflict from a stale start timestamp. See isStaleStartTs for what "stale" means.
func conflictAbortReason(stale bool) string {
	if stale {
		return abortReason(abortReasonStaleStartTs, abortDetailStaleStartTs)
	}
	return abortReason(abortReasonConflict, abortDetailConflict)
}

// lateAbortReason returns the wire reason for an abort decided after the commit timestamp was
// leased, and false if nothing aborted the transaction. orcErr is Oracle.commit's verdict and ctxErr
// is the request context's error, read after it. Two unrelated causes can abort here. An oracle
// rejection takes precedence: the oracle would have aborted the transaction whatever the context
// said, so that is the honest cause to report.
func lateAbortReason(orcErr, ctxErr error) (string, bool) {
	switch {
	case orcErr != nil:
		// Oracle.commit re-runs the conflict check, and a start timestamp that went stale since the
		// early check in commit() (a lease renewal or snapshot purge in between) is not a write-write
		// conflict. Oracle.commit reports that case as errStaleStartTs so it can be told apart.
		return conflictAbortReason(errors.Is(orcErr, errStaleStartTs)), true
	case ctxErr != nil:
		// The context was cancelled or timed out; no conflict occurred. No published category
		// describes this, so report the context's own message with no category rather than claiming
		// a conflict that did not happen. The gRPC status code stays codes.Aborted because the
		// transaction genuinely did abort; switching to Canceled or DeadlineExceeded would change
		// retry behaviour for clients that already treat Aborted as retryable.
		return abortReason(abortReasonUncategorized, ctxErr.Error()), true
	}
	return "", false
}

// isStaleStartTs reports whether this transaction's start timestamp (src.StartTs) is below the
// floor Zero can still validate against (startTxnTs). hasConflict aborts such a transaction, but
// the cause is not a write-write conflict, so it is reported under its own category. See
// abortDetailStaleStartTs for the two ways that floor rises. Callers must hold at least a read
// lock on the oracle.
func (o *Oracle) isStaleStartTs(src *api.TxnContext) bool {
	return src.StartTs < o.startTxnTs
}

// checkPreds reports whether any predicate this transaction touched is unusable, in which case the
// transaction must be aborted. It returns the abort category alongside the error because its exits
// are unrelated causes, not degrees of one cause: only a group mismatch and a blocked tablet are
// predicate moves. A malformed entry in preds, or a predicate that no group serves, are different
// failures needing different remedies — retrying a malformed predicate key can never succeed — so
// those return abortReasonUncategorized rather than claiming a move that did not happen. Every exit
// still aborts the transaction, exactly as before; only the reported category changes.
//
// Each entry in preds is a "<group id>-<namespaced predicate>" string built by the Alpha
// (LocalCache.fillPreds).
func (s *Server) checkPreds(preds []string) (string, error) {
	for _, pkey := range preds {
		splits := strings.SplitN(pkey, "-", 2)
		if len(splits) < 2 {
			return abortReasonUncategorized, errors.Errorf(abortDetailMissingGroupIDFmt, pkey)
		}
		// ParseUint with bitSize 32 rather than Atoi: group ids are uint32 everywhere, so this
		// bounds the value to the type it is compared against below and rejects negatives, which
		// Atoi accepted and the uint32 conversion silently wrapped (CodeQL go/incorrect-integer-conversion).
		gid, err := strconv.ParseUint(splits[0], 10, 32)
		if err != nil {
			return abortReasonUncategorized, errors.Wrapf(err, abortDetailBadGroupIDFmt, pkey)
		}
		pred := splits[1]
		if strings.Contains(pred, hnsw.VecKeyword) {
			pred = pred[0:strings.Index(pred, hnsw.VecKeyword)]
		}
		// Checked before the tablet lookup because blockTablet holds for the entire duration of a
		// move: movePredicate defers its unblock past the tablet reassignment proposal, so this is
		// the authoritative signal that a move is in flight. Checking it first means an in-flight
		// move can never be reported as anything else, and it lets the tablet == nil case below mean
		// only "no group serves this predicate" and never "it is moving".
		if s.isBlocked(pred) {
			return abortReasonPredicateMove, errors.Errorf(abortDetailPredicateBlockedFmt,
				displayPred(pred))
		}
		tablet := s.ServingTablet(pred)
		if tablet == nil {
			return abortReasonUncategorized, errors.Errorf(abortDetailTabletNilFmt, displayPred(pred))
		}
		// The predicate finished moving to another group while this transaction was open: the
		// mutation was written against group gid, but the predicate now belongs elsewhere.
		if tablet.GroupId != uint32(gid) {
			return abortReasonPredicateMove, errors.Errorf(abortDetailGroupMismatchFmt,
				gid, displayPred(pred), tablet.GroupId)
		}
	}
	return abortReasonUncategorized, nil
}

// displayPred turns a namespaced attribute ("0-name") into the predicate name the user wrote
// ("name") for use in an abort detail. Preds is client-supplied, so unlike x.ParseAttr this must not
// assume the separator is present: a missing separator or an empty name returns the input as is.
func displayPred(pred string) string {
	if _, attr, ok := strings.Cut(pred, x.NsSeparator); ok && attr != "" {
		return attr
	}
	return pred
}

func (s *Server) commit(ctx context.Context, src *api.TxnContext) error {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.Int64("startTs", int64(src.StartTs)))
	if src.Aborted {
		// The client discarded this transaction itself (a Discard call), so the server decided
		// nothing and there is no reason to report.
		return s.proposeTxn(ctx, src)
	}

	// abortWithReason marks the transaction aborted, proposes it (advancing the watermark exactly as
	// the previous code did), and then surfaces the categorized reason to the caller as a gRPC
	// Aborted status. proposeTxn still runs identically — only the post-propose return changes.
	abortWithReason := func(reason string) error {
		span.SetAttributes(attribute.Bool("abort", true))
		src.Aborted = true
		if err := s.proposeTxn(ctx, src); err != nil {
			return err
		}
		return status.Error(codes.Aborted, reason)
	}

	// Use the start timestamp to check if we have a conflict, before we need to assign a commit ts.
	s.orc.RLock()
	conflict := s.orc.hasConflict(src)
	// hasConflict also aborts a transaction whose start timestamp is too old to validate, which is
	// not a write-write conflict, so it is reported under its own category. See isStaleStartTs.
	stale := s.orc.isStaleStartTs(src)
	s.orc.RUnlock()
	if conflict {
		return abortWithReason(conflictAbortReason(stale))
	}

	if reason, err := s.checkPreds(src.Preds); err != nil {
		// checkPreds already builds a specific, human-readable message for each cause. Forward it
		// instead of discarding it, tagged with whichever category that cause justifies.
		return abortWithReason(abortReason(reason, err.Error()))
	}

	num := pb.Num{Val: 1, Type: pb.Num_TXN_TS}
	assigned, err := s.lease(ctx, &num)
	if err != nil {
		return err
	}
	src.CommitTs = assigned.StartId
	// Mark the transaction as done, irrespective of whether the proposal succeeded or not.
	defer s.orc.doneUntil.Done(src.CommitTs)
	span.SetAttributes(attribute.Int64("commitTs", int64(src.CommitTs)))
	span.SetAttributes(attribute.Int64("nodeId", int64(s.Node.Id)))
	span.AddEvent(fmt.Sprintf("TXN Context: %+v", src))

	// Past this point the transaction already holds a commit timestamp, so an abort here is a "late"
	// abort. See lateAbortReason for how its cause is reported.
	reason, aborted := lateAbortReason(s.orc.commit(src), ctx.Err())
	if aborted {
		span.SetAttributes(attribute.Bool("abort", true))
		src.Aborted = true
	}
	// Propose txn should be used to set watermark as done.
	if err := s.proposeTxn(ctx, src); err != nil {
		return err
	}
	if aborted {
		return status.Error(codes.Aborted, reason)
	}
	// proposeTxn sets src.Aborted when it finds no commit timestamp for this transaction, meaning
	// something aborted it out of band while this commit was in flight: a TryAbort from a schema
	// update, a drop-predicate, or the idle-transaction reaper, or a concurrent client Discard of the
	// same transaction. Nothing above set `aborted`, because
	// this commit itself decided to proceed. Returning nil here (as this did previously) loses the
	// abort entirely: CommitOverNetwork then sees tctx.Aborted with no error and falls through to a
	// bare dgo.ErrAborted, so the caller learns nothing. Report it instead.
	if src.Aborted {
		span.SetAttributes(attribute.Bool("abort", true))
		return status.Error(codes.Aborted,
			abortReason(abortReasonUncategorized, abortDetailPreAborted))
	}
	return nil
}

// CommitOrAbort either commits a transaction or aborts it.
// The abortion can happen under the following conditions
// 1) the api.TxnContext.Aborted flag is set in the src argument
// 2) if there's an error (e.g server is not the leader or there's a conflicting transaction)
func (s *Server) CommitOrAbort(ctx context.Context, src *api.TxnContext) (*api.TxnContext, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	ctx, span := otel.Tracer("").Start(ctx, "Zero.CommitOrAbort")
	defer span.End()

	if !s.Node.AmLeader() {
		return nil, errors.Errorf("Only leader can decide to commit or abort")
	}
	err := s.commit(ctx, src)
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
	}
	return src, err
}

var errClosed = errors.New("Streaming closed by oracle")
var errNotLeader = errors.New("Node is no longer leader")

// Oracle streams the oracle state to the alphas.
// The first entry sent by Zero contains the entire state of transactions. Zero periodically
// confirms receipt from the group, and truncates its state. This 2-way acknowledgement is a
// safe way to get the status of all the transactions.
func (s *Server) Oracle(_ *api.Payload, server pb.Zero_OracleServer) error {
	if !s.Node.AmLeader() {
		return errNotLeader
	}
	ch, id := s.orc.newSubscriber()
	defer s.orc.removeSubscriber(id)

	ctx := server.Context()
	leaderChangeCh := s.leaderChangeChannel()
	for {
		select {
		case <-leaderChangeCh:
			return errNotLeader
		case delta, open := <-ch:
			if !open {
				return errClosed
			}
			// Pass in the latest group checksum as well, so the Alpha can use that to determine
			// when not to service a read.
			delta.GroupChecksums = s.groupChecksums()
			if err := server.Send(&delta); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-s.closer.HasBeenClosed():
			return errServerShutDown
		}
	}
}

// TryAbort attempts to abort the given transactions which are not already committed..
func (s *Server) TryAbort(ctx context.Context,
	txns *pb.TxnTimestamps) (*pb.OracleDelta, error) {
	delta := &pb.OracleDelta{}
	for _, startTs := range txns.Ts {
		// Do via proposals to avoid race
		tctx := &api.TxnContext{StartTs: startTs, Aborted: true}
		if err := s.proposeTxn(ctx, tctx); err != nil {
			return delta, err
		}
		// Txn should be aborted if not already committed.
		delta.Txns = append(delta.Txns, &pb.TxnStatus{
			StartTs:  startTs,
			CommitTs: s.orc.commitTs(startTs)})
	}
	return delta, nil
}

// Timestamps is used to assign startTs for a new transaction
func (s *Server) Timestamps(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	ctx, span := otel.Tracer("").Start(ctx, "Zero.Timestamps")
	defer span.End()

	span.SetAttributes(attribute.Int64("zeroId", int64(s.Node.Id)))
	span.SetAttributes(attribute.String("timestampRequest", fmt.Sprintf("%+v", num)))
	if ctx.Err() != nil {
		return &emptyAssignedIds, ctx.Err()
	}

	num.Type = pb.Num_TXN_TS
	reply, err := s.lease(ctx, num)
	span.AddEvent(fmt.Sprintf("Response: %+v, Error: %v", reply, err))

	switch err {
	case nil:
		s.orc.doneUntil.Done(x.Max(reply.EndId, reply.ReadOnly))
		go s.orc.storePending(reply)
	case errServedFromMemory:
		// Avoid calling doneUntil.Done, and storePending.
		err = nil
	default:
		glog.Errorf("Got error: %v while leasing timestamps: %+v", err, num)
	}
	return reply, err
}
