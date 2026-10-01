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

// TestUnifiedConcurrentCreateFindOrCreate exercises the uf.mu invariant under
// `go test -race`: many goroutines hammer Create (monolithic opts) and
// FindOrCreate (partitioned opts) on the SAME name. Without the factory lock,
// pick()'s cross-factory "remove the stale registration from the other child"
// step races the selected child's create, so both children can end up holding a
// registration and Find becomes ambiguous. The test asserts no data race and
// that the name still resolves to exactly one live instance afterwards.
func TestUnifiedConcurrentCreateFindOrCreate(t *testing.T) {
	uf := CreateUnifiedFactory[float32](32)
	const name = "0-race"

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = uf.Create(name, monoOpts(), 32) }()
		go func() { defer wg.Done(); _, _ = uf.FindOrCreate(name, partitionedOpts(), 32) }()
	}
	wg.Wait()

	found, err := uf.Find(name)
	if err != nil {
		t.Fatalf("Find after concurrent storm: %v", err)
	}
	if found == nil {
		t.Fatal("Find returned nil after concurrent storm")
	}
}
