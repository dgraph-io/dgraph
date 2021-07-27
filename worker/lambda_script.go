/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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

package worker

import (
	"sync"
)

var lambdaScriptStore *LambdaScriptStore

type LambdaScript struct {
	ID     string `json:"id,omitempty"`
	Script string `json:"script,omitempty"`
}

type LambdaScriptStore struct {
	sync.RWMutex
	script map[uint64]*LambdaScript
}

func init() {
	lambdaScriptStore = &LambdaScriptStore{
		script: make(map[uint64]*LambdaScript),
	}
}

func Lambda() *LambdaScriptStore {
	return lambdaScriptStore
}

func (ls *LambdaScriptStore) Set(ns uint64, scr *LambdaScript) {
	ls.Lock()
	defer ls.Unlock()
	ls.script[ns] = scr
}

func (ls *LambdaScriptStore) GetCurrent(ns uint64) (*LambdaScript, bool) {
	ls.RLock()
	defer ls.RUnlock()
	scr, ok := ls.script[ns]
	return scr, ok
}

func (ls *LambdaScriptStore) resetLambdaScript() {
	ls.Lock()
	defer ls.Unlock()
	ls.script = make(map[uint64]*LambdaScript)
}

func ResetLambdaScriptStore() {
	lambdaScriptStore.resetLambdaScript()
}

func GetLambdaScript(ns uint64) string {
	if script, ok := lambdaScriptStore.GetCurrent(ns); ok {
		return script.Script
	}
	return ""
}
