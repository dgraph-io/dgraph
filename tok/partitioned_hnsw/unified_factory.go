/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package partitioned_hnsw

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	c "github.com/dgraph-io/dgraph/v25/tok/constraints"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	"github.com/dgraph-io/dgraph/v25/tok/index"
	opt "github.com/dgraph-io/dgraph/v25/tok/options"
)

// unifiedHNSWFactory exposes a single user-facing index type, "hnsw", and
// dispatches to the monolithic or the partitioned implementation based on
// the numClusters option: absent => today's plain hnsw, unchanged;
// numClusters > 1 => the partitioned (IVF-over-HNSW) implementation.
type unifiedHNSWFactory[T c.Float] struct {
	mono index.IndexFactory[T] // plain hnsw
	part index.IndexFactory[T] // partitioned
}

// CreateUnifiedFactory returns the factory registered under the "hnsw" name.
func CreateUnifiedFactory[T c.Float](floatBits int) index.IndexFactory[T] {
	return &unifiedHNSWFactory[T]{
		mono: hnsw.CreateFactory[T](floatBits),
		part: CreateFactory[T](floatBits),
	}
}

func (uf *unifiedHNSWFactory[T]) Name() string { return hnsw.Hnsw }

// SpecHasOption reports whether the stored index spec carries the given
// option key. Used to recognize partitioned specs now that both
// implementations share the "hnsw" name.
func SpecHasOption(spec *pb.VectorIndexSpec, key string) bool {
	for _, o := range spec.Options {
		if o.Key == key {
			return true
		}
	}
	return false
}

// NumClustersFromSpec returns the cluster count of a partitioned spec and
// whether the spec is partitioned. Returns (1000, true) as the default for
// partitioned specs with no explicit numClusters option. Returns (0, false)
// for non-partitioned specs.
func NumClustersFromSpec(spec *pb.VectorIndexSpec) (int, bool) {
	if !SpecHasOption(spec, NumClustersOpt) {
		return 0, false
	}
	for _, o := range spec.Options {
		if o.Key == NumClustersOpt {
			n, err := strconv.Atoi(o.Value)
			if err != nil || n < 1 {
				return 1000, true
			}
			return n, true
		}
	}
	return 1000, true
}

// isPartitioned reports whether o selects the partitioned implementation,
// validating the option combination. numClusters must be > 1 (a 1-cluster
// partitioned index is strictly worse than plain hnsw), and the
// partitioned-only tuning options are rejected without numClusters instead
// of being silently ignored.
func (uf *unifiedHNSWFactory[T]) isPartitioned(o opt.Options) (bool, error) {
	val, ok, err := opt.GetOpt(o, NumClustersOpt, 0)
	if err != nil {
		return false, err
	}
	if !ok {
		for _, dependent := range []string{NumProbesOpt, PartitionStratOpt, vectorDimension} {
			if _, present := opt.GetInterfaceOpt(o, dependent); present {
				return false, fmt.Errorf("%s requires numClusters (the partitioned index); "+
					"add numClusters or remove %s", dependent, dependent)
			}
		}
		return false, nil
	}
	if val <= 1 {
		return false, errors.New("numClusters must be greater than 1; omit it for a non-partitioned index")
	}
	return true, nil
}

// pick returns the factory selected by o, removing any stale registration of
// the same name from the other factory (a schema alter can flip a predicate
// between monolithic and partitioned; Find must never return the stale one).
func (uf *unifiedHNSWFactory[T]) pick(name string, o opt.Options) (index.IndexFactory[T], error) {
	partitioned, err := uf.isPartitioned(o)
	if err != nil {
		return nil, err
	}
	if partitioned {
		glog.V(1).Infof("vector index %s: numClusters set — using the partitioned "+
			"(experimental) implementation", name)
		_ = uf.mono.Remove(name)
		return uf.part, nil
	}
	_ = uf.part.Remove(name)
	return uf.mono, nil
}

func (uf *unifiedHNSWFactory[T]) Create(name string, o opt.Options, floatBits int) (index.VectorIndex[T], error) {
	f, err := uf.pick(name, o)
	if err != nil {
		return nil, err
	}
	return f.Create(name, o, floatBits)
}

func (uf *unifiedHNSWFactory[T]) CreateOrReplace(name string, o opt.Options, floatBits int) (index.VectorIndex[T], error) {
	f, err := uf.pick(name, o)
	if err != nil {
		return nil, err
	}
	return f.CreateOrReplace(name, o, floatBits)
}

func (uf *unifiedHNSWFactory[T]) FindOrCreate(name string, o opt.Options, floatBits int) (index.VectorIndex[T], error) {
	f, err := uf.pick(name, o)
	if err != nil {
		return nil, err
	}
	return f.FindOrCreate(name, o, floatBits)
}

func (uf *unifiedHNSWFactory[T]) Find(name string) (index.VectorIndex[T], error) {
	if vi, err := uf.part.Find(name); err != nil {
		return nil, err
	} else if vi != nil {
		return vi, nil
	}
	return uf.mono.Find(name)
}

func (uf *unifiedHNSWFactory[T]) Remove(name string) error {
	if err := uf.part.Remove(name); err != nil {
		return err
	}
	return uf.mono.Remove(name)
}

// GetOptions builds the identity string used by needsVectorIndexEdgesRebuild.
// CRITICAL: for options without numClusters this MUST be byte-identical to
// plain hnsw's output (hnsw.GetPersistantOptions), or every existing hnsw
// predicate re-indexes on upgrade. The partitioned factory's GetOptions
// already delegates to hnsw.GetPersistantOptions and appends
// numClusters/partitionStratOpt only when explicitly present, so it is
// correct for BOTH cases.
func (uf *unifiedHNSWFactory[T]) GetOptions(o opt.Options) string {
	return uf.part.GetOptions(o)
}

// AllowedOptions is the union: plain hnsw options plus the partitioned ones.
// The partitioned factory's AllowedOptions already contains exactly this
// union (exponent, maxLevels, efConstruction, efSearch, metric, numClusters,
// numProbes, partitionStratOpt, vectorDimension).
func (uf *unifiedHNSWFactory[T]) AllowedOptions() opt.AllowedOptions {
	return uf.part.AllowedOptions()
}
