/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import (
	"math"
	"sync"

	"github.com/hypermodeinc/dgraph/v25/algo"
)

type StatsHolder struct {
	sync.RWMutex

	predStats map[string]StatContainer
}

func NewStatsHolder() *StatsHolder {
	return &StatsHolder{
		predStats: make(map[string]StatContainer),
	}

}

type StatContainer interface {
	InsertRecord([]byte, uint64)
	Estimate([]byte) uint64
}

type EqContainer struct {
	sync.RWMutex

	cmf *algo.CountMinSketch
}

func NewEqContainer() *EqContainer {
	return &EqContainer{cmf: algo.NewCountMinSketch(0.001, 0.99)}
}

func (eq *EqContainer) Estimate(key []byte) uint64 {
	eq.RLock()
	defer eq.RUnlock()

	return eq.cmf.Count(key)
}

func (eq *EqContainer) InsertRecord(key []byte, count uint64) {
	eq.Lock()
	defer eq.Unlock()
	eq.cmf.AddInt(key, count)
}

func (sh *StatsHolder) InsertRecord(pred string, key []byte, count uint64) {
	sh.RLock()
	val, ok := sh.predStats[pred]
	sh.RUnlock()
	if !ok {
		return
	}

	val.InsertRecord(key, count)
}

func (sh *StatsHolder) ProcessEqPredicate(pred string, key []byte) uint64 {
	sh.RLock()
	val, ok := sh.predStats[pred]
	sh.RUnlock()
	if ok {
		return val.Estimate(key)
	}

	sh.Lock()
	val = NewEqContainer()
	sh.predStats[pred] = val
	sh.Unlock()
	return math.MaxUint64
}
