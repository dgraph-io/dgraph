/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package partitioned_hnsw

import (
	"sync"
	"testing"

	opt "github.com/dgraph-io/dgraph/v25/tok/options"
)

func testOptions() opt.Options {
	o := opt.NewOptions()
	o.SetOpt(NumClustersOpt, 4)
	return o
}

// TestFindOrCreateReturnsSameInstance pins the state-lifetime contract:
// mutation/query paths (FindOrCreate) share one long-lived instance, and a
// rebuild (CreateOrReplace) swaps it for a fresh one.
func TestFindOrCreateReturnsSameInstance(t *testing.T) {
	f := CreateFactory[float32](32)

	first, err := f.FindOrCreate("0-pred", testOptions(), 32)
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	second, err := f.FindOrCreate("0-pred", testOptions(), 32)
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	if first != second {
		t.Fatal("FindOrCreate returned a different instance for the same name")
	}

	rebuilt, err := f.CreateOrReplace("0-pred", testOptions(), 32)
	if err != nil {
		t.Fatalf("CreateOrReplace: %v", err)
	}
	if rebuilt == first {
		t.Fatal("CreateOrReplace must produce a fresh instance")
	}
	after, err := f.FindOrCreate("0-pred", testOptions(), 32)
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	if after != rebuilt {
		t.Fatal("FindOrCreate must return the rebuilt instance after CreateOrReplace")
	}
}

// TestFindOrCreateAppliesNumProbesChange pins the numProbes fix: a
// numProbes-only alter must NOT rebuild (numProbes is excluded from the index
// identity), so FindOrCreate returns the same long-lived instance — and it
// applies the new probe count in place rather than ignoring it.
func TestFindOrCreateAppliesNumProbesChange(t *testing.T) {
	f := CreateFactory[float32](32)

	o1 := opt.NewOptions()
	o1.SetOpt(NumClustersOpt, 4)
	o1.SetOpt(NumProbesOpt, 1)
	first, err := f.FindOrCreate("0-pred", o1, 32)
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}

	o2 := opt.NewOptions()
	o2.SetOpt(NumClustersOpt, 4)
	o2.SetOpt(NumProbesOpt, 3)
	second, err := f.FindOrCreate("0-pred", o2, 32)
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}

	if first != second {
		t.Fatal("numProbes change unexpectedly replaced the instance (should be rebuild-free)")
	}
	ph, ok := second.(*partitionedHNSW[float32])
	if !ok {
		t.Fatalf("unexpected index type %T", second)
	}
	if got := ph.resolveNumProbes(o2); got != 3 {
		t.Fatalf("new numProbes not resolved on the cached instance: got %d, want 3", got)
	}
}

// TestFindOrCreateConcurrent pins that concurrent callers all get the same
// instance with no duplicate creation (run with -race).
func TestFindOrCreateConcurrent(t *testing.T) {
	f := CreateFactory[float32](32)

	const workers = 32
	results := make([]any, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			vi, err := f.FindOrCreate("0-pred", testOptions(), 32)
			if err != nil {
				t.Errorf("FindOrCreate: %v", err)
				return
			}
			results[slot] = vi
		}(i)
	}
	wg.Wait()

	for i := 1; i < workers; i++ {
		if results[i] != results[0] {
			t.Fatalf("worker %d got a different instance", i)
		}
	}
}
