/*
 * Copyright 2024 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package posting

import (
	"math"
	"sync"

	boom "github.com/harshil-goel/BoomFilters"
)

var (
	GlobalStatsHolder *StatsHolder
)

type StatsHolder struct {
	sync.RWMutex

	predStats map[string]StatContainer
	//computeRequests chan *ComputeStatRequest
}

//type ComputeStatRequest struct {
//	pred string
//}

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

	cmf *boom.CountMinSketch
}

func NewEqContainer() *EqContainer {
	return &EqContainer{cmf: boom.NewCountMinSketch(0.001, 0.99)}
}

func (eq *EqContainer) Estimate(key []byte) uint64 {
	eq.RLock()
	defer eq.RUnlock()

	return eq.cmf.Count(key)
}

func (eq *EqContainer) InsertRecord(key []byte, count uint64) {
	eq.Lock()
	defer eq.Unlock()
	eq.cmf.Set(key, count)
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

func (sh *StatsHolder) ProcessPredicate(pred string, key []byte) uint64 {
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
