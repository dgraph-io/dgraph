# Entity-Level Sub-Tablet Routing — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan
> task-by-task.

**Goal:** Enable entity-level routing so that a UID's `dgraph.label` value pins ALL predicates for
that UID to the group assigned that label, using composite sub-tablet keys (`predicate@label`).

**Architecture:** Extend the existing predicate-tablet system with composite keys
(`predicate@label`). Zero's state machine gains multi-sub-tablet awareness. Mutations resolve entity
labels before routing. Queries fan out to all authorized sub-tablets for a predicate. The existing
predicate-level `@label` continues to work and acts as a fallback when no entity label exists.

**Tech Stack:** Go, Protocol Buffers, Raft consensus (via Zero), Badger (storage), dgo (Go client),
Docker Compose (integration tests).

**Design doc:** `docs/plans/2026-02-04-entity-level-sub-tablet-routing-design.md`

---

## Task 1: Add `tabletKey()` and `parseTabletKey()` helpers to `protos/pb/labeled.go`

These are the foundational helpers that encode/decode the composite key format `predicate@label`.
Every subsequent task depends on these.

**Files:**

- Modify: `protos/pb/labeled.go` (existing, currently lines 1-25)
- Create: `protos/pb/labeled_test.go`

**Step 1: Write the failing tests**

Create `protos/pb/labeled_test.go`:

```go
package pb

import "testing"

func TestTabletKey_Unlabeled(t *testing.T) {
	got := TabletKey("Document.name", "")
	if got != "Document.name" {
		t.Errorf("TabletKey('Document.name', '') = %q, want 'Document.name'", got)
	}
}

func TestTabletKey_Labeled(t *testing.T) {
	got := TabletKey("Document.name", "secret")
	if got != "Document.name@secret" {
		t.Errorf("TabletKey('Document.name', 'secret') = %q, want 'Document.name@secret'", got)
	}
}

func TestParseTabletKey_Unlabeled(t *testing.T) {
	pred, label := ParseTabletKey("Document.name")
	if pred != "Document.name" || label != "" {
		t.Errorf("ParseTabletKey('Document.name') = (%q, %q), want ('Document.name', '')", pred, label)
	}
}

func TestParseTabletKey_Labeled(t *testing.T) {
	pred, label := ParseTabletKey("Document.name@secret")
	if pred != "Document.name" || label != "secret" {
		t.Errorf("ParseTabletKey('Document.name@secret') = (%q, %q), want ('Document.name', 'secret')", pred, label)
	}
}

func TestParseTabletKey_NamespacedLabeled(t *testing.T) {
	// Dgraph namespaces predicates as "0-Document.name" — the '@' should still
	// be the delimiter even with the namespace prefix.
	pred, label := ParseTabletKey("0-Document.name@top_secret")
	if pred != "0-Document.name" || label != "top_secret" {
		t.Errorf("ParseTabletKey('0-Document.name@top_secret') = (%q, %q), want ('0-Document.name', 'top_secret')", pred, label)
	}
}

func TestTabletKeyRoundTrip(t *testing.T) {
	cases := []struct{ pred, label string }{
		{"Document.name", ""},
		{"Document.name", "secret"},
		{"0-Document.name", "top_secret"},
		{"dgraph.type", ""},
	}
	for _, c := range cases {
		key := TabletKey(c.pred, c.label)
		gotPred, gotLabel := ParseTabletKey(key)
		if gotPred != c.pred || gotLabel != c.label {
			t.Errorf("Round-trip(%q, %q): got (%q, %q)", c.pred, c.label, gotPred, gotLabel)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/mwelles/Developer/dgraph-io/dgraph && go test ./protos/pb/ -run TestTabletKey -v`
Expected: FAIL — `TabletKey` and `ParseTabletKey` are undefined.

**Step 3: Write minimal implementation**

Add to `protos/pb/labeled.go` (after existing `SchemaUpdate.IsLabeled` at line 25):

```go
import "strings"

const tabletKeySep = "@"

// TabletKey returns the composite key for a sub-tablet. Unlabeled sub-tablets
// use the bare predicate name for backward compatibility.
func TabletKey(predicate, label string) string {
	if label == "" {
		return predicate
	}
	return predicate + tabletKeySep + label
}

// ParseTabletKey splits a composite tablet key into its predicate and label
// components. For keys without a label (no '@' separator), the label is "".
func ParseTabletKey(key string) (predicate, label string) {
	if idx := strings.LastIndex(key, tabletKeySep); idx >= 0 {
		return key[:idx], key[idx+1:]
	}
	return key, ""
}
```

Note: We use `strings.LastIndex` because the `@` character is not valid in Dgraph predicate names
(allowed chars: `a-zA-Z0-9_.~-` where `-` is only used for the namespace prefix like `0-`). However,
`LastIndex` is safer than `Index` as a defensive choice.

**Step 4: Run test to verify it passes**

Run: `cd /Users/mwelles/Developer/dgraph-io/dgraph && go test ./protos/pb/ -run TestTabletKey -v`
Expected: PASS — all 6 tests pass.

**Step 5: Commit**

```bash
git add protos/pb/labeled.go protos/pb/labeled_test.go
git commit -m "feat(sharding): add TabletKey/ParseTabletKey composite key helpers"
```

---

## Task 2: Add `ServingSubTablet()` and `ServingTablets()` to Zero

Zero's state machine currently has `ServingTablet(predicate)` which does a direct map lookup by
predicate name. We need two new functions:

- `ServingSubTablet(pred, label)` — finds the ONE group serving a specific `(predicate, label)` pair
- `ServingTablets(pred)` — finds ALL sub-tablets for a predicate across all groups (for query
  fan-out)

**Files:**

- Modify: `dgraph/cmd/zero/zero.go` (lines 308-340)

**Step 1: Write `servingSubTablet` (internal, expects caller holds read lock)**

Add after `servingTablet` (currently at line 333) in `dgraph/cmd/zero/zero.go`:

```go
// servingSubTablet returns the tablet for the given (predicate, label) pair.
// For unlabeled sub-tablets, the key is just the predicate name.
// For labeled sub-tablets, the key is "predicate@label".
// Caller must hold at least a read lock.
func (s *Server) servingSubTablet(predicate, label string) *pb.Tablet {
	s.AssertRLock()
	key := pb.TabletKey(predicate, label)
	for _, group := range s.state.Groups {
		if tab, ok := group.Tablets[key]; ok {
			return tab
		}
	}
	return nil
}
```

**Step 2: Write `ServingSubTablet` (public, acquires its own lock)**

Add after `ServingTablet` (currently at line 308) in `dgraph/cmd/zero/zero.go`:

```go
// ServingSubTablet returns the tablet for the given (predicate, label) pair.
// For labeled sub-tablets the map key is "predicate@label".
// For unlabeled sub-tablets the key is the bare predicate name.
func (s *Server) ServingSubTablet(predicate, label string) *pb.Tablet {
	s.RLock()
	defer s.RUnlock()
	return s.servingSubTablet(predicate, label)
}
```

**Step 3: Write `ServingTablets` (returns all sub-tablets for a predicate)**

Add after `ServingSubTablet` in `dgraph/cmd/zero/zero.go`:

```go
// ServingTablets returns all sub-tablets for a given predicate across all groups.
// This includes both the unlabeled sub-tablet (key = predicate) and any labeled
// sub-tablets (key = predicate@label). Used for query fan-out.
func (s *Server) ServingTablets(predicate string) []*pb.Tablet {
	s.RLock()
	defer s.RUnlock()

	var tablets []*pb.Tablet
	for _, group := range s.state.Groups {
		for key, tab := range group.Tablets {
			tabPred, _ := pb.ParseTabletKey(key)
			if tabPred == predicate {
				tablets = append(tablets, tab)
			}
		}
	}
	return tablets
}
```

**Step 4: Verify compilation**

Run: `cd /Users/mwelles/Developer/dgraph-io/dgraph && go build ./dgraph/cmd/zero/` Expected:
Compiles successfully.

**Step 5: Commit**

```bash
git add dgraph/cmd/zero/zero.go
git commit -m "feat(sharding): add ServingSubTablet and ServingTablets to Zero state machine"
```

---

## Task 3: Update `handleTablet` to use composite keys

The current `handleTablet` in `raft.go:313` stores tablets using `tablet.Predicate` as the map key.
For sub-tablet routing, we need to store them using `TabletKey(predicate, label)`. The
duplicate-detection check changes from "is anyone serving this predicate?" to "is anyone serving
this (predicate, label) pair?".

**Files:**

- Modify: `dgraph/cmd/zero/raft.go` (lines 313-355)

**Step 1: Update `handleTablet` to use composite keys**

The key changes in `raft.go:313-355`:

1. Line 322: `delete(group.Tablets, tablet.Predicate)` → use composite key
2. Line 334: `n.server.servingTablet(tablet.Predicate)` → use `servingSubTablet`
3. Line 337: `delete(originalGroup.Tablets, tablet.Predicate)` → use composite key
4. Line 344: `delete(originalGroup.Tablets, tablet.Predicate)` → use composite key
5. Line 353: `group.Tablets[tablet.Predicate] = tablet` → use composite key

Replace the entire `handleTablet` function:

```go
func (n *node) handleTablet(tablet *pb.Tablet) error {
	state := n.server.state
	if tablet.GroupId == 0 {
		return errors.Errorf("Tablet group id is zero: %+v", tablet)
	}

	key := pb.TabletKey(tablet.Predicate, tablet.Label)

	group := state.Groups[tablet.GroupId]
	if tablet.Remove {
		glog.Infof("Removing tablet for key: [%v], gid: [%v]\n", key, tablet.GroupId)
		if group != nil {
			delete(group.Tablets, key)
		}
		return nil
	}
	if group == nil {
		group = newGroup()
		state.Groups[tablet.GroupId] = group
	}

	// Duplicate detection: check if this (predicate, label) pair is already served.
	// Multiple groups CAN serve the same predicate as long as they have different labels.
	if prev := n.server.servingSubTablet(tablet.Predicate, tablet.Label); prev != nil {
		if tablet.Force {
			originalGroup := state.Groups[prev.GroupId]
			delete(originalGroup.Tablets, key)
		} else if tablet.IsLabeled() && prev.Label != tablet.Label {
			// Allow re-routing when labels differ. This happens when a schema with @label
			// is applied after the predicate was created without a label.
			glog.Infof("Tablet for key: [%s] re-routing from group %d to %d due to label change (%q -> %q)",
				key, prev.GroupId, tablet.GroupId, prev.Label, tablet.Label)
			originalGroup := state.Groups[prev.GroupId]
			delete(originalGroup.Tablets, key)
		} else if prev.GroupId != tablet.GroupId {
			glog.Infof(
				"Tablet for key: [%s], gid: [%d] already served by group: [%d]\n",
				key, tablet.GroupId, prev.GroupId)
			return errTabletAlreadyServed
		}
	}
	tablet.Force = false
	group.Tablets[key] = tablet
	return nil
}
```

**Step 2: Verify compilation**

Run: `cd /Users/mwelles/Developer/dgraph-io/dgraph && go build ./dgraph/cmd/zero/` Expected:
Compiles successfully.

**Step 3: Commit**

```bash
git add dgraph/cmd/zero/raft.go
git commit -m "feat(sharding): update handleTablet to use composite sub-tablet keys"
```

---

## Task 4: Update `chooseTablet` rebalancer to handle composite keys

The rebalancer in `tablet.go:227` iterates `group.Tablets` and uses `tab.Predicate` as the return
value. With composite keys, the map key is now `predicate@label`, but `tab.Predicate` is still the
bare predicate. The rebalancer already skips labeled tablets via `tab.IsLabeled()`. We just need to
ensure it works correctly with the new key format.

**Files:**

- Modify: `dgraph/cmd/zero/tablet.go` (lines 227-296)

**Step 1: Review and update `chooseTablet`**

The `chooseTablet` function at line 246 does:

```go
for _, tab := range v.Tablets {
    space += tab.OnDiskBytes
}
```

This still works because it iterates values, not keys.

At line 275, it does:

```go
for _, tab := range group.Tablets {
    if x.IsReservedPredicate(tab.Predicate) { continue }
    if tab.IsLabeled() { continue }
    ...
    predicate = tab.Predicate
```

This also works because `tab.Predicate` is the bare predicate name and `tab.IsLabeled()` correctly
skips labeled sub-tablets. **No changes needed to `chooseTablet`.**

However, `movePredicate` at line 139 does:

```go
tab := s.ServingTablet(predicate)
```

And `ServingTablet` (line 308) iterates `group.Tablets[tablet]` using the bare predicate name. With
composite keys, `ServingTablet("Document.name")` will still find the unlabeled sub-tablet
`"Document.name"` (since unlabeled keys use the bare predicate). This is correct — `movePredicate`
only moves unlabeled tablets (because `chooseTablet` skips labeled ones).

**No code changes needed.** Verify:

Run: `cd /Users/mwelles/Developer/dgraph-io/dgraph && go build ./dgraph/cmd/zero/` Expected:
Compiles successfully.

**Step 2: Commit (skip if no changes)**

If no changes were needed, skip the commit. Otherwise:

```bash
git add dgraph/cmd/zero/tablet.go
git commit -m "refactor(sharding): verify rebalancer works with composite sub-tablet keys"
```

---

## Task 5: Register `dgraph.label` as a reserved predicate

`dgraph.label` needs to be recognized as a reserved predicate so it always lives on group 1 and
can't be moved or rebalanced. Currently `IsReservedPredicate` checks for the `dgraph.` prefix (in
`x/keys.go:700`), so `dgraph.label` is already reserved by convention. But we should register it as
a **pre-defined** predicate with a schema entry so the system knows its type.

**Files:**

- Modify: `x/keys.go` — add `dgraph.label` to the pre-defined predicates list
- Modify: `schema/schema.go` or wherever initial schema is defined — add `dgraph.label: string .`

**Step 1: Find the pre-defined predicates list**

Search for where pre-defined predicates like `dgraph.type` are registered. This is typically in the
initial schema definition.

Run: `grep -rn "dgraph.type" x/keys.go | head -5` to find the pattern.

**Step 2: Add `dgraph.label` to the pre-defined predicates list**

Add `"dgraph.label"` to the `preDefinedPredicateMap` in `x/keys.go` (near line 730, where
`dgraph.type` and ACL predicates are listed).

**Step 3: Add initial schema definition for `dgraph.label`**

Find where `dgraph.type` gets its initial schema entry (likely in `schema/schema.go` or
`worker/groups.go` initial schema) and add:

```
dgraph.label: string @index(exact) .
```

The `@index(exact)` allows efficient lookups by label value (e.g., "find all UIDs with
label=secret"), which is useful for reclassification enumeration.

**Step 4: Verify compilation and that existing tests still pass**

Run: `cd /Users/mwelles/Developer/dgraph-io/dgraph && go build ./...` Expected: Compiles
successfully.

**Step 5: Commit**

```bash
git add x/keys.go schema/schema.go
git commit -m "feat(sharding): register dgraph.label as pre-defined reserved predicate"
```

---

## Task 6: Add entity label cache to worker

Implement an in-memory `UID -> label` cache that the mutation routing layer uses to resolve entity
labels without hitting group 1 on every mutation.

**Files:**

- Create: `worker/entity_label_cache.go`
- Create: `worker/entity_label_cache_test.go`

**Step 1: Write the failing tests**

Create `worker/entity_label_cache_test.go`:

```go
package worker

import "testing"

func TestEntityLabelCache_GetSet(t *testing.T) {
	c := newEntityLabelCache(100)
	c.Set(42, "secret")
	label, ok := c.Get(42)
	if !ok || label != "secret" {
		t.Errorf("Get(42) = (%q, %v), want ('secret', true)", label, ok)
	}
}

func TestEntityLabelCache_Miss(t *testing.T) {
	c := newEntityLabelCache(100)
	label, ok := c.Get(99)
	if ok || label != "" {
		t.Errorf("Get(99) = (%q, %v), want ('', false)", label, ok)
	}
}

func TestEntityLabelCache_Invalidate(t *testing.T) {
	c := newEntityLabelCache(100)
	c.Set(42, "secret")
	c.Invalidate(42)
	label, ok := c.Get(42)
	if ok {
		t.Errorf("Get(42) after Invalidate = (%q, %v), want ('', false)", label, ok)
	}
}

func TestEntityLabelCache_Clear(t *testing.T) {
	c := newEntityLabelCache(100)
	c.Set(1, "a")
	c.Set(2, "b")
	c.Clear()
	if _, ok := c.Get(1); ok {
		t.Error("Get(1) after Clear should miss")
	}
	if _, ok := c.Get(2); ok {
		t.Error("Get(2) after Clear should miss")
	}
}

func TestEntityLabelCache_UnlabeledEntity(t *testing.T) {
	// An entity with no label should be cached as "" (empty string)
	// so we don't repeatedly look it up from group 1.
	c := newEntityLabelCache(100)
	c.Set(42, "")
	label, ok := c.Get(42)
	if !ok || label != "" {
		t.Errorf("Get(42) = (%q, %v), want ('', true)", label, ok)
	}
}
```

**Step 2: Run test to verify it fails**

Run:
`cd /Users/mwelles/Developer/dgraph-io/dgraph && go test ./worker/ -run TestEntityLabelCache -v`
Expected: FAIL — `newEntityLabelCache` is undefined.

**Step 3: Write minimal implementation**

Create `worker/entity_label_cache.go`:

```go
/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import "sync"

// entityLabelCache is a concurrency-safe UID -> label cache.
// Used by the mutation routing layer to resolve entity labels without
// querying group 1 on every mutation.
type entityLabelCache struct {
	mu      sync.RWMutex
	entries map[uint64]string
	maxSize int
}

func newEntityLabelCache(maxSize int) *entityLabelCache {
	return &entityLabelCache{
		entries: make(map[uint64]string),
		maxSize: maxSize,
	}
}

// Get returns the cached label for a UID. Returns ("", false) on cache miss.
// An empty label with ok=true means the entity is explicitly unlabeled.
func (c *entityLabelCache) Get(uid uint64) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	label, ok := c.entries[uid]
	return label, ok
}

// Set stores a UID -> label mapping. If the cache exceeds maxSize, it is
// cleared (simple eviction strategy — revisit with LRU if needed).
func (c *entityLabelCache) Set(uid uint64, label string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxSize {
		// Simple eviction: clear everything. This is acceptable because
		// cache misses just cause a read from group 1, not data loss.
		c.entries = make(map[uint64]string)
	}
	c.entries[uid] = label
}

// Invalidate removes a single UID from the cache.
func (c *entityLabelCache) Invalidate(uid uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, uid)
}

// Clear removes all entries. Used on DropAll.
func (c *entityLabelCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[uint64]string)
}
```

**Step 4: Run test to verify it passes**

Run:
`cd /Users/mwelles/Developer/dgraph-io/dgraph && go test ./worker/ -run TestEntityLabelCache -v`
Expected: PASS — all 5 tests pass.

**Step 5: Commit**

```bash
git add worker/entity_label_cache.go worker/entity_label_cache_test.go
git commit -m "feat(sharding): add entity label cache for UID -> label lookups"
```

---

## Task 7: Two-phase mutation routing in `populateMutationMap`

This is the core change. `populateMutationMap` (at `worker/mutation.go:705`) currently routes edges
by predicate label only. We need to add Phase 1 (scan for `dgraph.label` edges) and Phase 2 (resolve
entity label before predicate label).

**Files:**

- Modify: `worker/mutation.go` (lines 705-765)

**Step 1: Add `resolveEntityLabel` and `resolveLabel` functions**

Add before `populateMutationMap` in `worker/mutation.go`:

```go
// Global entity label cache, initialized during group setup.
var elCache *entityLabelCache

func initEntityLabelCache() {
	elCache = newEntityLabelCache(1_000_000) // 1M entries ~= 16MB
}

// resolveEntityLabel returns the entity-level label for a UID.
// Priority: batch labels > cache > read from group 1.
func resolveEntityLabel(uid uint64, batchLabels map[uint64]string) string {
	if label, ok := batchLabels[uid]; ok {
		return label
	}
	if elCache != nil {
		if label, ok := elCache.Get(uid); ok {
			return label
		}
	}
	// TODO: Cache miss — read dgraph.label from group 1.
	// For now, return "" (unlabeled). The group-1 read will be added
	// in a follow-up task once the integration test cluster is running.
	return ""
}

// resolveLabel determines the effective label for routing an edge.
// Priority: entity label > predicate @label > unlabeled.
func resolveLabel(uid uint64, predicate string, batchLabels map[uint64]string) string {
	if label := resolveEntityLabel(uid, batchLabels); label != "" {
		return label
	}
	label, _ := schema.State().GetLabel(context.Background(), predicate)
	return label
}
```

**Step 2: Update `populateMutationMap` with two-phase routing**

Replace the data-mutation loop (lines 707-722) with:

```go
func populateMutationMap(src *pb.Mutations) (map[uint32]*pb.Mutations, error) {
	mm := make(map[uint32]*pb.Mutations)

	// PHASE 1: Scan for dgraph.label edges to build entity -> label map.
	// This handles new entities whose labels are set in the same mutation batch.
	batchLabels := make(map[uint64]string)
	for _, edge := range src.Edges {
		pred, _ := pb.ParseTabletKey(edge.Attr)
		if pred == "dgraph.label" || x.ParseAttr(pred) == "dgraph.label" {
			batchLabels[edge.Entity] = string(edge.Value)
		}
	}

	// PHASE 2: Route each edge using the entity's resolved label.
	for _, edge := range src.Edges {
		attr := edge.Attr
		pred, _ := pb.ParseTabletKey(attr)

		var label string
		if x.IsReservedPredicate(pred) {
			// Reserved predicates (dgraph.label, dgraph.type, ACL) always use
			// predicate-level routing (typically group 1).
			label, _ = schema.State().GetLabel(context.Background(), attr)
		} else {
			// Non-reserved predicates use entity-label-aware resolution.
			label = resolveLabel(edge.Entity, attr, batchLabels)
		}

		gid, err := groups().BelongsTo(attr, label)
		if err != nil {
			return nil, err
		}

		mu := mm[gid]
		if mu == nil {
			mu = &pb.Mutations{GroupId: gid}
			mm[gid] = mu
		}
		mu.Edges = append(mu.Edges, edge)
		mu.Metadata = src.Metadata
	}

	// Schema mutations — unchanged, use predicate-level label.
	for _, schemaUpdate := range src.Schema {
		gid, err := groups().BelongsTo(schemaUpdate.Predicate, schemaUpdate.Label)
		if err != nil {
			return nil, err
		}

		mu := mm[gid]
		if mu == nil {
			mu = &pb.Mutations{GroupId: gid}
			mm[gid] = mu
		}
		mu.Schema = append(mu.Schema, schemaUpdate)
	}

	if src.DropOp > 0 {
		for _, gid := range groups().KnownGroups() {
			mu := mm[gid]
			if mu == nil {
				mu = &pb.Mutations{GroupId: gid}
				mm[gid] = mu
			}
			mu.DropOp = src.DropOp
			mu.DropValue = src.DropValue
		}
	}

	if len(src.Types) > 0 {
		for _, gid := range groups().KnownGroups() {
			mu := mm[gid]
			if mu == nil {
				mu = &pb.Mutations{GroupId: gid}
				mm[gid] = mu
			}
			mu.Types = src.Types
		}
	}

	return mm, nil
}
```

**Step 3: Verify compilation**

Run: `cd /Users/mwelles/Developer/dgraph-io/dgraph && go build ./worker/` Expected: Compiles
successfully.

**Step 4: Commit**

```bash
git add worker/mutation.go
git commit -m "feat(sharding): two-phase entity-label-aware mutation routing in populateMutationMap"
```

---

## Task 8: Update `BelongsToReadOnly` for composite keys (query path)

`BelongsToReadOnly` (at `worker/groups.go:408`) is used by `ProcessTaskOverNetwork` for query
routing. Currently it looks up tablets by bare predicate name. For sub-tablet routing, the query
path needs to support fan-out to multiple sub-tablets. However, the first step is to ensure
single-sub-tablet lookups still work correctly.

The query path changes are more complex and will be split across Tasks 8 and 9.

**Files:**

- Modify: `worker/groups.go` (lines 408-446)

**Step 1: Add `AllTablets` function for query fan-out**

Add after `BelongsToReadOnly` in `worker/groups.go`:

```go
// AllSubTablets returns all cached sub-tablets for a predicate.
// This is used for query fan-out when a predicate has multiple sub-tablets.
// Returns nil if only a single sub-tablet exists (fast path).
func (g *groupi) AllSubTablets(predicate string, ts uint64) ([]*pb.Tablet, error) {
	g.RLock()
	var tablets []*pb.Tablet
	for key, tablet := range g.tablets {
		tabPred, _ := pb.ParseTabletKey(key)
		if tabPred == predicate {
			if ts > 0 && ts < tablet.MoveTs {
				g.RUnlock()
				return nil, errors.Errorf("StartTs: %d is from before MoveTs: %d for pred: %q",
					ts, tablet.MoveTs, key)
			}
			tablets = append(tablets, tablet)
		}
	}
	g.RUnlock()

	if len(tablets) <= 1 {
		// Single sub-tablet or no sub-tablets — handled by normal BelongsToReadOnly path.
		return nil, nil
	}
	return tablets, nil
}
```

**Step 2: Verify compilation**

Run: `cd /Users/mwelles/Developer/dgraph-io/dgraph && go build ./worker/` Expected: Compiles
successfully.

**Step 3: Commit**

```bash
git add worker/groups.go
git commit -m "feat(sharding): add AllSubTablets for query fan-out lookup"
```

---

## Task 9: Query fan-out in `ProcessTaskOverNetwork`

Update `ProcessTaskOverNetwork` (at `worker/task.go:124`) to scatter queries across multiple
sub-tablets when a predicate has more than one sub-tablet.

**Files:**

- Modify: `worker/task.go` (lines 124-160)

**Step 1: Add result merging helper**

Add before `ProcessTaskOverNetwork` in `worker/task.go`:

```go
// mergeResults combines results from multiple sub-tablet queries.
// Each sub-tablet returns results only for UIDs it has postings for.
func mergeResults(results []*pb.Result) *pb.Result {
	if len(results) == 0 {
		return &pb.Result{}
	}
	if len(results) == 1 {
		return results[0]
	}

	merged := &pb.Result{}
	// Merge UID matrices: each result has one UidMatrix entry per query UID.
	// For fan-out, all results have the same number of UidMatrix entries.
	// Merge by appending UIDs from each sub-tablet's response.
	if len(results[0].UidMatrix) > 0 {
		merged.UidMatrix = make([]*pb.List, len(results[0].UidMatrix))
		for i := range merged.UidMatrix {
			merged.UidMatrix[i] = &pb.List{}
		}
		for _, r := range results {
			for i, list := range r.UidMatrix {
				if i < len(merged.UidMatrix) {
					merged.UidMatrix[i].Uids = append(merged.UidMatrix[i].Uids, list.Uids...)
				}
			}
		}
	}

	// Merge value matrices similarly.
	if len(results[0].ValueMatrix) > 0 {
		merged.ValueMatrix = make([]*pb.ValueList, len(results[0].ValueMatrix))
		for i := range merged.ValueMatrix {
			merged.ValueMatrix[i] = &pb.ValueList{}
		}
		for _, r := range results {
			for i, vl := range r.ValueMatrix {
				if i < len(merged.ValueMatrix) {
					merged.ValueMatrix[i].Values = append(merged.ValueMatrix[i].Values, vl.Values...)
				}
			}
		}
	}

	// Merge counts.
	if len(results[0].Counts) > 0 {
		merged.Counts = make([]uint32, len(results[0].Counts))
		for _, r := range results {
			for i, c := range r.Counts {
				if i < len(merged.Counts) {
					merged.Counts[i] += c
				}
			}
		}
	}

	// IntersectDest is not relevant for fan-out queries.
	// LinRead is not relevant for fan-out queries.
	return merged
}
```

**Step 2: Update `ProcessTaskOverNetwork` to support fan-out**

Replace `ProcessTaskOverNetwork` in `worker/task.go`:

```go
func ProcessTaskOverNetwork(ctx context.Context, q *pb.Query) (*pb.Result, error) {
	attr := q.Attr

	// Check for multi-sub-tablet fan-out.
	subTablets, err := groups().AllSubTablets(attr, q.ReadTs)
	if err != nil {
		return nil, err
	}

	if len(subTablets) > 1 {
		// Fan-out path: send query to all sub-tablet groups in parallel.
		return processTaskFanOut(ctx, q, subTablets)
	}

	// Fast path: single sub-tablet (or none), use existing routing.
	gid, err := groups().BelongsToReadOnly(attr, q.ReadTs)
	switch {
	case err != nil:
		return nil, err
	case gid == 0:
		return nil, errNonExistentTablet
	}

	span := trace.SpanFromContext(ctx)
	span.AddEvent("ProcessTaskOverNetwork", trace.WithAttributes(
		attribute.String("attr", attr),
		attribute.String("gid", fmt.Sprintf("%d", gid)),
		attribute.String("readTs", fmt.Sprintf("%d", q.ReadTs)),
		attribute.String("node_id", fmt.Sprintf("%d", groups().Node.Id))))

	if groups().ServesGroup(gid) {
		return processTask(ctx, q, gid)
	}

	result, err := processWithBackupRequest(ctx, gid,
		func(ctx context.Context, c pb.WorkerClient) (interface{}, error) {
			return c.ServeTask(ctx, q)
		})
	if err != nil {
		return nil, err
	}

	reply := result.(*pb.Result)
	span.AddEvent("Reply from server", trace.WithAttributes(
		attribute.Int("len", len(reply.UidMatrix)),
		attribute.Int64("gid", int64(gid)),
		attribute.String("attr", attr)))
	return reply, nil
}

// processTaskFanOut sends the query to all sub-tablet groups in parallel
// and merges the results.
func processTaskFanOut(ctx context.Context, q *pb.Query, subTablets []*pb.Tablet) (*pb.Result, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("ProcessTaskFanOut", trace.WithAttributes(
		attribute.String("attr", q.Attr),
		attribute.Int("sub_tablets", len(subTablets)),
		attribute.String("readTs", fmt.Sprintf("%d", q.ReadTs))))

	type fanOutResult struct {
		result *pb.Result
		err    error
	}

	ch := make(chan fanOutResult, len(subTablets))
	for _, tab := range subTablets {
		gid := tab.GroupId
		go func(gid uint32) {
			if groups().ServesGroup(gid) {
				r, err := processTask(ctx, q, gid)
				ch <- fanOutResult{r, err}
				return
			}
			r, err := processWithBackupRequest(ctx, gid,
				func(ctx context.Context, c pb.WorkerClient) (interface{}, error) {
					return c.ServeTask(ctx, q)
				})
			if err != nil {
				ch <- fanOutResult{nil, err}
				return
			}
			ch <- fanOutResult{r.(*pb.Result), nil}
		}(gid)
	}

	var results []*pb.Result
	for range subTablets {
		r := <-ch
		if r.err != nil {
			return nil, r.err
		}
		results = append(results, r.result)
	}

	merged := mergeResults(results)
	span.AddEvent("FanOut merged", trace.WithAttributes(
		attribute.Int("result_count", len(results)),
		attribute.String("attr", q.Attr)))
	return merged, nil
}
```

**Step 3: Verify compilation**

Run: `cd /Users/mwelles/Developer/dgraph-io/dgraph && go build ./worker/` Expected: Compiles
successfully.

**Step 4: Commit**

```bash
git add worker/task.go
git commit -m "feat(sharding): query fan-out across sub-tablets in ProcessTaskOverNetwork"
```

---

## Task 10: Sort fan-out in `SortOverNetwork`

Apply the same fan-out pattern to `SortOverNetwork` (at `worker/sort.go:48`).

**Files:**

- Modify: `worker/sort.go` (lines 48-77)

**Step 1: Update `SortOverNetwork` for fan-out**

Replace `SortOverNetwork`:

```go
func SortOverNetwork(ctx context.Context, q *pb.SortMessage) (*pb.SortResult, error) {
	attr := q.Order[0].Attr

	// Check for multi-sub-tablet fan-out.
	subTablets, err := groups().AllSubTablets(attr, q.ReadTs)
	if err != nil {
		return &emptySortResult, err
	}

	if len(subTablets) > 1 {
		return processSortFanOut(ctx, q, subTablets)
	}

	// Fast path: single sub-tablet.
	gid, err := groups().BelongsToReadOnly(attr, q.ReadTs)
	if err != nil {
		return &emptySortResult, err
	} else if gid == 0 {
		return &emptySortResult,
			errors.Errorf("Cannot sort by unknown attribute %s", x.ParseAttr(attr))
	}

	if span := trace.SpanFromContext(ctx); span != nil {
		span.SetAttributes(
			attribute.String("attribute", attr),
			attribute.Int("groupId", int(gid)),
		)
	}

	if groups().ServesGroup(gid) {
		return processSort(ctx, q)
	}

	result, err := processWithBackupRequest(
		ctx, gid, func(ctx context.Context, c pb.WorkerClient) (interface{}, error) {
			return c.Sort(ctx, q)
		})
	if err != nil {
		return &emptySortResult, err
	}
	return result.(*pb.SortResult), nil
}

func processSortFanOut(ctx context.Context, q *pb.SortMessage, subTablets []*pb.Tablet) (*pb.SortResult, error) {
	type fanOutResult struct {
		result *pb.SortResult
		err    error
	}

	ch := make(chan fanOutResult, len(subTablets))
	for _, tab := range subTablets {
		gid := tab.GroupId
		go func(gid uint32) {
			if groups().ServesGroup(gid) {
				r, err := processSort(ctx, q)
				ch <- fanOutResult{r, err}
				return
			}
			r, err := processWithBackupRequest(ctx, gid,
				func(ctx context.Context, c pb.WorkerClient) (interface{}, error) {
					return c.Sort(ctx, q)
				})
			if err != nil {
				ch <- fanOutResult{nil, err}
				return
			}
			ch <- fanOutResult{r.(*pb.SortResult), nil}
		}(gid)
	}

	var results []*pb.SortResult
	for range subTablets {
		r := <-ch
		if r.err != nil {
			return &emptySortResult, r.err
		}
		results = append(results, r.result)
	}

	return mergeSortResults(results, q), nil
}

func mergeSortResults(results []*pb.SortResult, q *pb.SortMessage) *pb.SortResult {
	if len(results) == 0 {
		return &emptySortResult
	}
	if len(results) == 1 {
		return results[0]
	}

	// Merge UID matrices from all sub-tablets.
	merged := &pb.SortResult{}
	if len(results[0].UidMatrix) > 0 {
		merged.UidMatrix = make([]*pb.List, len(results[0].UidMatrix))
		for i := range merged.UidMatrix {
			merged.UidMatrix[i] = &pb.List{}
		}
		for _, r := range results {
			for i, list := range r.UidMatrix {
				if i < len(merged.UidMatrix) {
					merged.UidMatrix[i].Uids = append(merged.UidMatrix[i].Uids, list.Uids...)
				}
			}
		}
	}
	return merged
}
```

**Step 2: Verify compilation**

Run: `cd /Users/mwelles/Developer/dgraph-io/dgraph && go build ./worker/` Expected: Compiles
successfully.

**Step 3: Commit**

```bash
git add worker/sort.go
git commit -m "feat(sharding): sort fan-out across sub-tablets in SortOverNetwork"
```

---

## Task 11: Integration test — entity-level routing end-to-end

Write an integration test that verifies the full entity-level routing flow: set `dgraph.label` on
entities, mutate predicates, and verify they land on the correct groups.

**Files:**

- Modify: `systest/label/label_test.go`

**Step 1: Write the integration test**

Add to `systest/label/label_test.go`:

```go
func TestEntityLevelRouting(t *testing.T) {
	waitForCluster(t)

	dg := testutil.DgraphClientWithGroot("localhost:9080")

	// Apply schema — no @label directives on predicates.
	// Routing should be determined by dgraph.label on entities.
	err := dg.Alter(context.Background(), &api.Operation{
		DropAll: true,
	})
	require.NoError(t, err)

	err = dg.Alter(context.Background(), &api.Operation{
		Schema: `
			Document.name: string .
			Document.text: string .
			dgraph.label: string @index(exact) .
		`,
	})
	require.NoError(t, err)

	// Mutate: create entities with different labels.
	txn := dg.NewTxn()
	mu := &api.Mutation{
		SetNquads: []byte(`
			_:doc1 <dgraph.type> "Document" .
			_:doc1 <dgraph.label> "secret" .
			_:doc1 <Document.name> "Secret.pdf" .
			_:doc1 <Document.text> "Classified content" .

			_:doc2 <dgraph.type> "Document" .
			_:doc2 <dgraph.label> "top_secret" .
			_:doc2 <Document.name> "Top Secret.pdf" .
			_:doc2 <Document.text> "Highly classified content" .

			_:doc3 <dgraph.type> "Document" .
			_:doc3 <Document.name> "Boring.pdf" .
			_:doc3 <Document.text> "Unclassified memo" .
		`),
		CommitNow: true,
	}
	resp, err := txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify: check tablet assignments via Zero state.
	time.Sleep(5 * time.Second) // Allow tablet assignment to propagate.
	state, err := testutil.GetState()
	require.NoError(t, err)

	// Build a map of tablet key -> group ID.
	tabletToGroup := make(map[string]string)
	for groupID, group := range state.Groups {
		for tabletKey := range group.Tablets {
			tabletToGroup[tabletKey] = groupID
		}
	}

	// Expect sub-tablets for Document.name and Document.text:
	// - "0-Document.name"          on group 1 (unlabeled)
	// - "0-Document.name@secret"   on group 2 (secret)
	// - "0-Document.name@top_secret" on group 3 (top_secret)
	t.Logf("Tablet assignments: %+v", tabletToGroup)

	// Find which group has the "secret" label.
	labelToGroup := make(map[string]string)
	for groupID, group := range state.Groups {
		for _, member := range group.Members {
			if member.Label != "" {
				labelToGroup[member.Label] = groupID
			}
		}
	}

	secretGroup := labelToGroup["secret"]
	topSecretGroup := labelToGroup["top_secret"]
	require.NotEmpty(t, secretGroup, "should have a group with label 'secret'")
	require.NotEmpty(t, topSecretGroup, "should have a group with label 'top_secret'")

	// Verify sub-tablet assignments.
	require.Equal(t, secretGroup, tabletToGroup["0-Document.name@secret"],
		"Document.name@secret should be on the secret group")
	require.Equal(t, topSecretGroup, tabletToGroup["0-Document.name@top_secret"],
		"Document.name@top_secret should be on the top_secret group")

	// Verify query returns all documents.
	queryResp, err := dg.NewReadOnlyTxn().Query(context.Background(), `{
		docs(func: type(Document)) {
			uid
			Document.name
			Document.text
		}
	}`)
	require.NoError(t, err)
	t.Logf("Query response: %s", queryResp.GetJson())
	// Should return all 3 documents despite data living on 3 different groups.
}
```

**Step 2: Run integration test**

Run: Build dgraph binaries, start the label test cluster, and run the test:

```bash
cd /Users/mwelles/Developer/dgraph-io/dgraph && make install
cd systest/label && docker compose up -d
go test -tags=integration -v -run TestEntityLevelRouting ./systest/label/
```

Expected: Test passes — mutations route to correct groups, query fan-out returns all documents.

**Step 3: Commit**

```bash
git add systest/label/label_test.go
git commit -m "test(sharding): add entity-level routing integration test"
```

---

## Task 12: Entity label cache — group 1 read on cache miss

Complete the `resolveEntityLabel` function from Task 7 to actually read `dgraph.label` from group 1
on cache miss, instead of returning "".

**Files:**

- Modify: `worker/mutation.go` (the `resolveEntityLabel` function)

**Step 1: Implement group 1 read**

Update `resolveEntityLabel` in `worker/mutation.go`:

```go
func resolveEntityLabel(uid uint64, batchLabels map[uint64]string) string {
	if label, ok := batchLabels[uid]; ok {
		return label
	}
	if elCache != nil {
		if label, ok := elCache.Get(uid); ok {
			return label
		}
	}
	// Cache miss — read dgraph.label from wherever it's stored.
	// dgraph.label is a reserved predicate, so it follows normal predicate routing.
	label := readEntityLabelFromStore(uid)
	if elCache != nil {
		elCache.Set(uid, label)
	}
	return label
}

// readEntityLabelFromStore reads the dgraph.label value for a UID.
// This does a local posting list lookup if this group serves dgraph.label,
// or a network call to the serving group otherwise.
func readEntityLabelFromStore(uid uint64) string {
	ctx := context.Background()
	q := &pb.Query{
		Attr:   x.NamespaceAttr(x.RootNamespace, "dgraph.label"),
		UidList: &pb.List{Uids: []uint64{uid}},
		ReadTs: State.GetTimestamp(false),
	}
	result, err := ProcessTaskOverNetwork(ctx, q)
	if err != nil {
		glog.V(2).Infof("Failed to read dgraph.label for uid %d: %v", uid, err)
		return ""
	}
	if len(result.ValueMatrix) > 0 && len(result.ValueMatrix[0].Values) > 0 {
		val := result.ValueMatrix[0].Values[0]
		if len(val.Val) > 0 {
			return string(val.Val)
		}
	}
	return ""
}
```

**Step 2: Verify compilation**

Run: `cd /Users/mwelles/Developer/dgraph-io/dgraph && go build ./worker/` Expected: Compiles
successfully.

**Step 3: Commit**

```bash
git add worker/mutation.go
git commit -m "feat(sharding): implement group-1 read for entity label cache miss"
```

---

## Task 13: Entity label cache invalidation on DropAll

When `DropAll` occurs, the entity label cache must be cleared.

**Files:**

- Modify: `worker/mutation.go` — find where DropAll is handled and add cache clear

**Step 1: Find DropAll handler and add cache invalidation**

Search for DropAll handling in the worker package and add `elCache.Clear()` at the appropriate
point.

**Step 2: Verify compilation and commit**

```bash
git add worker/mutation.go
git commit -m "feat(sharding): clear entity label cache on DropAll"
```

---

## Summary of tasks and dependencies

```
Task 1: TabletKey/ParseTabletKey helpers       ← foundation, no deps
Task 2: ServingSubTablet/ServingTablets (Zero)  ← depends on Task 1
Task 3: handleTablet composite keys             ← depends on Task 1, 2
Task 4: Verify rebalancer (may be no-op)        ← depends on Task 3
Task 5: Register dgraph.label as reserved       ← independent
Task 6: Entity label cache                      ← independent
Task 7: Two-phase mutation routing              ← depends on Task 1, 5, 6
Task 8: AllSubTablets query lookup              ← depends on Task 1
Task 9: ProcessTaskOverNetwork fan-out          ← depends on Task 8
Task 10: SortOverNetwork fan-out                ← depends on Task 8
Task 11: Integration test                       ← depends on all above
Task 12: Group 1 read on cache miss             ← depends on Task 6, 7
Task 13: DropAll cache invalidation             ← depends on Task 6

Parallelizable groups:
  [1]       → [2, 5, 6, 8]  → [3, 7, 9, 10]  → [4, 11, 12, 13]
```
