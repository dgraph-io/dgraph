/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package partitioned_hnsw

import (
	"sync"
	"testing"

	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	opt "github.com/dgraph-io/dgraph/v25/tok/options"
)

func monoOpts() opt.Options {
	o := opt.NewOptions()
	o.SetOpt(hnsw.MaxLevelsOpt, 3)
	o.SetOpt(hnsw.EfConstructionOpt, 64)
	o.SetOpt(hnsw.EfSearchOpt, 32)
	return o
}

func partitionedOpts() opt.Options {
	o := monoOpts()
	o.SetOpt(NumClustersOpt, 4)
	return o
}

// TestUnifiedDispatch pins that the unified factory builds a plain hnsw index
// when numClusters is absent, and the partitioned implementation when it is
// present and > 1.
func TestUnifiedDispatch(t *testing.T) {
	uf := CreateUnifiedFactory[float32](32)

	mono, err := uf.Create("0-mono", monoOpts(), 32)
	if err != nil {
		t.Fatalf("Create (monolithic): %v", err)
	}
	if _, isPart := mono.(*partitionedHNSW[float32]); isPart {
		t.Fatal("expected a monolithic hnsw index when numClusters is absent, got partitioned")
	}

	part, err := uf.Create("0-part", partitionedOpts(), 32)
	if err != nil {
		t.Fatalf("Create (partitioned): %v", err)
	}
	if _, isPart := part.(*partitionedHNSW[float32]); !isPart {
		t.Fatal("expected a partitioned index when numClusters > 1")
	}
}

// TestUnifiedIdentityBackCompat is the critical backward-compatibility pin:
// for options WITHOUT numClusters, the unified factory's identity string
// (Name + GetOptions) must be byte-identical to the plain hnsw factory's, or
// every existing hnsw predicate would re-index on upgrade.
func TestUnifiedIdentityBackCompat(t *testing.T) {
	uf := CreateUnifiedFactory[float32](32)
	mono := hnsw.CreateFactory[float32](32)

	o := monoOpts()
	unifiedIdentity := uf.Name() + uf.GetOptions(o)
	monoIdentity := mono.Name() + mono.GetOptions(o)

	if unifiedIdentity != monoIdentity {
		t.Fatalf("identity mismatch for non-partitioned options:\n unified = %q\n mono    = %q\n"+
			"(existing hnsw predicates would re-index on upgrade)", unifiedIdentity, monoIdentity)
	}
}

// TestUnifiedNumClustersMustExceedOne pins the validation that a 1-cluster
// partitioned index is rejected (it is strictly worse than plain hnsw).
func TestUnifiedNumClustersMustExceedOne(t *testing.T) {
	uf := CreateUnifiedFactory[float32](32)
	o := monoOpts()
	o.SetOpt(NumClustersOpt, 1)

	if _, err := uf.Create("0-one", o, 32); err == nil {
		t.Fatal("expected an error for numClusters=1, got nil")
	}
}

// TestUnifiedPartitionedOptionRequiresNumClusters pins that a partitioned-only
// tuning option without numClusters is a clear error rather than silently
// ignored.
func TestUnifiedPartitionedOptionRequiresNumClusters(t *testing.T) {
	uf := CreateUnifiedFactory[float32](32)
	o := monoOpts()
	o.SetOpt(NumProbesOpt, 8)

	if _, err := uf.Create("0-probes", o, 32); err == nil {
		t.Fatal("expected an error for numProbes without numClusters, got nil")
	}
}

// TestUnifiedFlipTransition pins the stale-instance cleanup in pick(): a
// predicate altered from partitioned to monolithic must serve the monolithic
// instance afterwards, not the stale partitioned one via Find.
func TestUnifiedFlipTransition(t *testing.T) {
	uf := CreateUnifiedFactory[float32](32)

	if _, err := uf.FindOrCreate("0-flip", partitionedOpts(), 32); err != nil {
		t.Fatalf("FindOrCreate (partitioned): %v", err)
	}
	if _, err := uf.CreateOrReplace("0-flip", monoOpts(), 32); err != nil {
		t.Fatalf("CreateOrReplace (monolithic): %v", err)
	}

	found, err := uf.Find("0-flip")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found == nil {
		t.Fatal("Find returned nil after the flip")
	}
	if _, isPart := found.(*partitionedHNSW[float32]); isPart {
		t.Fatal("Find still returns the stale partitioned instance after altering to monolithic")
	}
}

// TestUnifiedConcurrentCreateFindOrCreate pins the uf.mu invariant: a name must
// end up registered in exactly ONE child factory. pick() removes the name from
// the other child before the selected child creates it, and the child locks
// guard each factory alone, not this cross-factory sequence. Without uf.mu a
// concurrent Create (monolithic opts) and FindOrCreate (partitioned opts) on the
// same name can both pick-remove before either creates, leaving BOTH children
// holding a registration.
//
// That double registration is not an unsynchronized memory access (each child
// has its own lock), so -race cannot flag it, and the unified Find returns
// non-nil either way. The test therefore inspects the children directly and,
// because a single check after the storm only reflects the last interleaving,
// runs the racing pair in rounds and requires exactly one child to hold the
// name after each round.
func TestUnifiedConcurrentCreateFindOrCreate(t *testing.T) {
	uf := CreateUnifiedFactory[float32](32)
	ufc := uf.(*unifiedHNSWFactory[float32])
	const name = "0-race"

	for i := 0; i < 50; i++ {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = uf.Create(name, monoOpts(), 32) }()
		go func() { defer wg.Done(); _, _ = uf.FindOrCreate(name, partitionedOpts(), 32) }()
		wg.Wait()

		m, _ := ufc.mono.Find(name)
		p, _ := ufc.part.Find(name)
		if (m == nil) == (p == nil) {
			t.Fatalf("round %d: expected exactly one child registration for %q; mono=%v part=%v",
				i, name, m != nil, p != nil)
		}
	}
}
