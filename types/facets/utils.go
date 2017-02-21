/*
 * Copyright 2017 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package facets

import (
	"sort"
	"strings"
)

func (a Facets) Len() int { return len(a.Facets) }
func (a Facets) Swap(i, j int) {
	a.Facets[i], a.Facets[j] = a.Facets[j], a.Facets[i]
}
func (a Facets) Less(i, j int) bool {
	return a.Facets[i].Key < a.Facets[j].Key
}

func SortFacets(fs []*Facet) {
	sort.Sort(Facets{fs})
}

// CopyFacets makes a copy of facets of the posting which are requested in param.Keys.
func CopyFacets(fcs []*Facet, param *Param) (fs []*Facet) {
	if len(fcs) == 0 {
		return []*Facet{}
	}
	// facets and param.keys are both sorted,
	// We also need all keys if param.AllKeys is true.
	numKeys := len(param.Keys)
	numFacets := len(fcs)
	for kidx, fidx := 0, 0; (param.AllKeys || kidx < numKeys) && fidx < numFacets; {
		f := fcs[fidx]
		if param.AllKeys || param.Keys[kidx] == f.Key {
			fcopy := &Facet{Key: f.Key, Value: nil, ValType: f.ValType}
			fcopy.Value = make([]byte, len(f.Value))
			copy(fcopy.Value, f.Value)
			fs = append(fs, fcopy)
			kidx++
			fidx++
		} else if f.Key > param.Keys[kidx] {
			kidx++
		} else {
			fidx++
		}
	}
	return fs
}

// FilterKeys keeps only param.Keys in fcs array.
// param.Keys and fcs should be sorted (as always).
func FilterKeys(param *Param, fcs []*Facet) []*Facet {
	if param.AllKeys {
		return fcs
	}
	numFacets := len(fcs)
	numKeys := len(param.Keys)
	writeIdx := 0
	for fi, ki := 0, 0; fi < numFacets && ki < numKeys; {
		scmp := strings.Compare(fcs[fi].Key, param.Keys[ki])
		if scmp == 0 {
			fcs[writeIdx] = fcs[fi]
			fi++
			ki++
			writeIdx++
		} else if scmp == -1 {
			fi++
		} else {
			ki++
		}
	}
	fcs = fcs[:writeIdx]
	return fcs
}
