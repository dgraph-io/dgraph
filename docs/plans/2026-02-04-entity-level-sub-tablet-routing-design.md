# Entity-Level Sub-Tablet Routing

**Date:** 2026-02-04 **Status:** Draft / Design **Branch:** sharding-poc **PR:** #9574

---

## Problem Statement

The current predicate-level `@label` routing pins an _entire predicate_ to a specific alpha group.
All UIDs for that predicate live on the same group. This is useful for field-level classification
("this field is always secret") but does not support entity-level classification ("this document is
secret").

Entity-level routing means that when a UID has `dgraph.label = "secret"`, **all predicates for that
UID** are stored on the group assigned the "secret" label. Different UIDs for the same predicate can
live on different groups depending on their entity label.

### Example

```rdf
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
```

Expected routing:

| Entity | Label      | `Document.name` stored on | `Document.text` stored on |
| ------ | ---------- | ------------------------- | ------------------------- |
| doc1   | secret     | group 2 (secret)          | group 2 (secret)          |
| doc2   | top_secret | group 3 (top_secret)      | group 3 (top_secret)      |
| doc3   | (none)     | group 1 (unlabeled)       | group 1 (unlabeled)       |

---

## Core Constraint

Dgraph's sharding unit is the **predicate tablet**. Zero's tablet map is `predicate -> group`. A
predicate can only be served by one group. Entity-level routing requires the same predicate to be
served by multiple groups simultaneously.

## Chosen Approach: Sub-Tablet Routing

Extend the tablet system so a single predicate can have multiple **sub-tablets**, each keyed by
`(predicate, label)` and assigned to a different group. No predicate renaming. The routing layer
becomes label-aware.

---

## Design

### 1. Entity Label Registry (`dgraph.label`)

`dgraph.label` is a **reserved predicate on group 1**, like `dgraph.type` and ACL predicates. It
maps `UID -> label string`.

**Query path does NOT need per-UID label lookups.** The query planner fans out to all authorized
sub-tablets. Each group returns only UIDs it stores. Label filtering is implicit in data
distribution.

**Mutation path needs the lookup.** Two cases:

- **New entity:** Extract label from the mutation batch itself (scan for `dgraph.label` edges before
  routing other edges).
- **Existing entity:** Look up from local cache. Cache miss reads from group 1.

**Caching:** Each alpha maintains a local `UID -> label` cache, populated on reads and mutations.
Invalidated when `dgraph.label` changes (triggers reclassification).

### 2. Composite Tablet Key

The tablet map key changes from `predicate` to `predicate@label` for labeled sub-tablets. Unlabeled
sub-tablets keep the bare predicate name for backward compatibility.

```go
func tabletKey(predicate, label string) string {
    if label == "" {
        return predicate              // "Document.name"
    }
    return predicate + "@" + label    // "Document.name@secret"
}
```

The `@` character is not valid in Dgraph predicate names (allowed chars: `a-zA-Z0-9_.~`), so
collisions are impossible.

**Tablet map examples:**

```
Group 1 tablets:
  "Document.name"              → unlabeled sub-tablet
  "dgraph.label"               → reserved, entity label storage
  "dgraph.type"                → reserved

Group 2 tablets:
  "Document.name@secret"       → secret sub-tablet
  "Document.text@secret"       → secret sub-tablet

Group 3 tablets:
  "Document.name@top_secret"   → top_secret sub-tablet
  "Document.text@top_secret"   → top_secret sub-tablet
```

**Key property:** Existing code that accesses `group.Tablets["Document.name"]` still works unchanged
— it matches the unlabeled sub-tablet. Only new label-aware code parses the `@` separator.

### 3. Zero State Machine Changes

**Three lookup functions replace one:**

| Function                        | Purpose                                                | Used By                   |
| ------------------------------- | ------------------------------------------------------ | ------------------------- |
| `ServingSubTablet(pred, label)` | Find the ONE group serving this (pred, label) pair     | Mutations, `handleTablet` |
| `ServingTablets(pred)`          | Find ALL sub-tablets for a predicate across groups     | Query fan-out             |
| `ServingTablet(pred)`           | **Backward compat** — returns the unlabeled sub-tablet | Existing code             |

**`handleTablet` change:** The duplicate-detection check changes from "is anyone serving this
predicate?" to "is anyone serving this (predicate, label) pair?". Multiple groups can serve the same
predicate as long as they have different labels.

**Rebalancer:** Sub-tablets with non-empty labels are pinned. Only unlabeled sub-tablets participate
in rebalancing.

### 4. Mutation Routing

`populateMutationMap` changes from predicate-based to entity-label-based routing.

**Two-phase approach:**

```
PHASE 1: Build entity -> label map from this mutation batch.
         Scan for dgraph.label edges (handles new entities).

PHASE 2: Route each edge using the entity's label.
         - dgraph.label edges always route to group 1 (reserved)
         - All other edges route to the entity's label group
```

**Label resolution priority:**

```
1. Entity label (dgraph.label)    → highest priority
2. Predicate label (@label schema) → fallback default
3. Neither                         → normal unlabeled routing
```

This means predicate-level `@label` acts as a default for predicates where entities don't have their
own labels. Entity-level `dgraph.label` is an override.

**`resolveLabel` function:**

```go
func resolveLabel(uid uint64, predicate string, batchLabels map[uint64]string) string {
    // 1. Entity label takes priority
    if label := resolveEntityLabel(uid, batchLabels); label != "" {
        return label
    }
    // 2. Fall back to predicate-level @label
    if label, ok := schema.State().GetLabel(ctx, predicate); ok {
        return label
    }
    // 3. Unlabeled
    return ""
}
```

**`resolveEntityLabel` function:**

```go
func resolveEntityLabel(uid uint64, batchLabels map[uint64]string) string {
    // 1. Check mutation batch (new entity)
    if label, ok := batchLabels[uid]; ok {
        return label
    }
    // 2. Check local cache
    if label, ok := entityLabelCache.Get(uid); ok {
        return label
    }
    // 3. Cache miss — read from group 1
    label, _ := readEntityLabel(uid)
    entityLabelCache.Set(uid, label)
    return label
}
```

**Mixed-label mutations work naturally.** A single mutation batch containing edges for entities with
different labels produces multiple group-specific mutation batches via `populateMutationMap`.

### 5. Query Fan-Out

`ProcessTaskOverNetwork` changes from single-group dispatch to multi-group scatter-gather.

**Fast path:** When a predicate has only one sub-tablet (common case for unlabeled predicates),
routing is identical to today — zero overhead.

**Fan-out path:** When multiple sub-tablets exist:

1. Look up all sub-tablets for the predicate.
2. Filter by auth context (only query labels the user can access).
3. Send the query (including full UID list) to each authorized sub-tablet in parallel.
4. Merge results.

**UID list handling:** Send the full UID list to all groups. Each group ignores UIDs it doesn't have
postings for. This avoids per-UID label lookups on the query path. Slight network overhead but much
simpler.

**Functions that need fan-out:**

| Function                             | Location              |
| ------------------------------------ | --------------------- |
| `ProcessTaskOverNetwork`             | `worker/task.go`      |
| `processSort`                        | `worker/sort.go`      |
| Internal callers in `query/query.go` | Benefit automatically |

**Index-backed functions** (`handleHasFunction`, `handleRegexFunction`, etc.) need no changes — by
the time they execute, the query is already scoped to one group's data.

### 6. Reclassification (Entity Label Changes)

When an entity's label changes, all its postings must migrate from the old group to the new group.
This follows the existing predicate-move pattern but scoped to a single entity.

**Synchronous, blocking migration** (consistent with how predicate moves work today).

**Sequence:**

```
1. DETECT:    Old label != new label for the entity
2. BLOCK:     Block mutations for this entity (per-entity block)
3. ENUMERATE: Query source group for all predicates where entity has postings
              (iterate group's tablet list, check each for the target UID)
4. MIGRATE:   For each predicate:
              a. Read postings for this UID from source group
              b. Write postings to destination group
              c. Delete from source group
5. UPDATE:    Write new dgraph.label on group 1
6. INVALIDATE: Clear entity label caches across alphas
7. UNBLOCK:   Resume mutations for this entity
```

**Data volume is small.** An entity typically has data across dozens of predicates, but each
predicate has only one posting for the UID. Migration should complete in milliseconds to seconds.

**Fence timestamp pattern:** Same as predicate moves — lease a timestamp from Zero before migration.
Queries with `readTs` before the fence see the old location; queries after see the new location.

### 7. Cross-Label Edges

Cross-label edges work naturally with no special handling.

```rdf
_:doc1 <dgraph.label> "secret" .
_:doc1 <Document.author> _:person1 .   # person1 is unlabeled
```

The edge posting `(Document.author, doc1) -> person1` is stored on group 2 (where doc1's data
lives). The target UID (person1) lives on group 1. Dgraph resolves cross-group UID references at
query time during graph traversal. This is existing behavior.

### 8. Edge Cases

**DropAll:**

- Deletes all data, tablets, and sub-tablets.
- Entity label cache is invalidated.
- Sub-tablets are recreated on re-schema + re-mutation.

**Backup / Restore:**

- Each group backs up its own sub-tablet data.
- Restore group 1 first (includes `dgraph.label` mappings).
- Sub-tablets are recreated via `ForceTablet` during restore.
- Entity label cache rebuilds naturally.

**Live Loader:**

- Uses `populateMutationMap` — benefits from two-phase routing automatically.

**Bulk Loader:**

- Needs similar two-phase logic in its map phase: scan for `dgraph.label` edges, then route by
  entity label.

---

## Coexistence with Predicate-Level @label

Entity-level routing coexists with the existing predicate-level `@label` directive. Both produce the
same sub-tablet key format (`predicate@label`).

| Aspect         | Predicate-level `@label`      | Entity-level `dgraph.label`  |
| -------------- | ----------------------------- | ---------------------------- |
| Label source   | Schema definition             | Entity data                  |
| Routing lookup | `schema.State().GetLabel()`   | `resolveEntityLabel()`       |
| Granularity    | Every UID for that predicate  | Every predicate for that UID |
| Use case       | "This field is always secret" | "This document is secret"    |

**Conflict resolution:** Entity label wins. If a predicate has `@label(secret)` and an entity has
`dgraph.label = "top_secret"`, the entity's label takes precedence.

---

## Files Affected (Estimated)

| Area               | Files                                             | Scope                                                                             |
| ------------------ | ------------------------------------------------- | --------------------------------------------------------------------------------- |
| Proto              | `protos/pb.proto`, `protos/pb/labeled.go`         | Add `tabletKey()` helper                                                          |
| Zero state machine | `dgraph/cmd/zero/zero.go`, `raft.go`, `tablet.go` | `ServingSubTablet`, `ServingTablets`, composite key in `handleTablet`, rebalancer |
| Worker routing     | `worker/groups.go`, `mutation.go`, `proposal.go`  | `populateMutationMap` two-phase, `BelongsTo` entity-label-aware                   |
| Query fan-out      | `worker/task.go`, `worker/sort.go`                | `ProcessTaskOverNetwork` scatter-gather, `mergeResults`                           |
| Entity label cache | `worker/groups.go` (new)                          | `entityLabelCache`, `resolveEntityLabel`                                          |
| Reclassification   | `worker/` (new file)                              | `reclassifyEntity`, per-entity blocking, migration                                |
| Schema interaction | `worker/mutation.go`                              | `resolveLabel` priority: entity > predicate > none                                |
| Online restore     | `worker/online_restore.go`                        | Pass entity labels during `ForceTablet`                                           |
| Tests              | `systest/label/`                                  | New entity-level routing and reclassification tests                               |

---

## Open Questions

1. **Entity label cache eviction policy.** LRU with max size? TTL? Bounded by namespace?
2. **Bulk loader support.** How deep should entity-label awareness go in the bulk loader's
   map/reduce phases?
3. **Metrics / observability.** What new metrics are needed for sub-tablet fan-out latency,
   reclassification duration, cache hit rates?
4. **`/moveTablet` API.** Should it accept a label parameter to move a specific sub-tablet? Or only
   operate on unlabeled tablets?
