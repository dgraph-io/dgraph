/*
* Copyright 2016 DGraph Labs, Inc.
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
* http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*
 */

package cluster

import (
	"bytes"

	"github.com/dgraph-io/dgraph/store"
)

func getPredicate(b []byte) string {
	buf := bytes.NewBuffer(b)
	a, _ := buf.ReadString('|')
	str := string(a[:len(a)-1]) // omit the trailing '|'
	return str
}

func GetPredicateList(ps *store.Store) []string {
	var predicateList []string
	var lastPredicate, predicate string

	it := ps.GetIterator()
	for it.SeekToFirst(); it.Valid(); it.Next() {
		predicate = getPredicate(it.Key())
		if predicate != lastPredicate {
			predicateList = append(predicateList, predicate)
			lastPredicate = predicate
		}
	}
	return predicateList
}
