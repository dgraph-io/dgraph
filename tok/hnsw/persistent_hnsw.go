package hnsw

import (
	"fmt"

	c "github.com/dgraph-io/dgraph/tok/constraints"
	opt "github.com/dgraph-io/dgraph/tok/options"
)

type persistentHNSW[T c.Float] struct {
	maxLevels      int
	efConstruction int
	efSearch       int
	pred           string
	vecEntryKey    string
	vecKey         string
	vecDead        string
	simType        SimilarityType[T]
	floatBits      int
}

func (ph *persistentHNSW[T]) applyOptions(o opt.Options) error {
	if o.Specifies(ExponentOpt) {
		// Adjust defaults based on exponent.
		exponent, _, _ := opt.GetOpt(o, ExponentOpt, 3)

		if !o.Specifies(MaxLevelsOpt) {
			o.SetOpt(MaxLevelsOpt, exponent)
		}

		if !o.Specifies(EfConstructionOpt) {
			o.SetOpt(EfConstructionOpt, 6*exponent)
		}

		if !o.Specifies(EfSearchOpt) {
			o.SetOpt(EfConstructionOpt, 9*exponent)
		}
	}

	var err error
	ph.maxLevels, _, err = opt.GetOpt(o, MaxLevelsOpt, 3)
	if err != nil {
		return err
	}
	ph.efConstruction, _, err = opt.GetOpt(o, EfConstructionOpt, 18)
	if err != nil {
		return err
	}
	ph.efSearch, _, err = opt.GetOpt(o, EfSearchOpt, 27)
	if err != nil {
		return err
	}
	simType, foundSimType := opt.GetInterfaceOpt(o, MetricOpt)
	if foundSimType {
		okSimType, ok := simType.(SimilarityType[T])
		if !ok {
			return fmt.Errorf("cannot cast %T to SimilarityType", simType)
		}
		ph.simType = okSimType
	} else {
		ph.simType = SimilarityType[T]{indexType: Euclidian, distanceScore: euclidianDistanceSq[T],
			insortHeap: insortPersistentHeapAscending[T], isBetterScore: isBetterScoreForDistance[T]}
	}
	return nil
}
