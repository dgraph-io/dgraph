/*
 * Copyright 2016 DGraph Labs, Inc.
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

package worker

import (
	"strings"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

type FuncType int

const (
	NotFn FuncType = iota
	AggregatorFn
	CompareAttrFn
	CompareScalarFn
	GeoFn
	PasswordFn
	StandardFn = 100
)

func ParseFuncType(arr []string) (FuncType, string) {
	if len(arr) == 0 {
		return NotFn, ""
	}
	f := strings.ToLower(arr[0])
	switch f {
	case "leq", "geq", "lt", "gt", "eq":
		// gt(release_date, "1990") is 'CompareAttr' which
		//    takes advantage of indexed-attr
		// gt(count(films), 0) is 'CompareScalar', we first do
		//    counting on attr, then compare the result as scalar with int
		if len(arr) > 2 && arr[1] == "count" {
			return CompareScalarFn, f
		}
		return CompareAttrFn, f
	case "min", "max", "sum":
		return AggregatorFn, f
	case "checkpwd":
		return PasswordFn, f
	default:
		if types.IsGeoFunc(f) {
			return GeoFn, f
		}
		return StandardFn, f
	}
}

// arguments will not be changed once initialized
type FuncArg struct {
	fname     string
	fnType    FuncType
	gid       uint32
	q         *task.Query
	opts      posting.ListOptions
	threshold int64
}

type Key struct {
	key []byte
	uid uint64
}

type FuncResult struct {
	Value  *task.Value
	Count  uint32
	Facets *facets.List
	List   *task.List
}

func processFunction(args *FuncArg, chin chan *Key, chout chan *FuncResult) {
	for elem := range chin {
		result := &FuncResult{}
		q, fname, fnType, opts := args.q, args.fname, args.fnType, args.opts

		// Get or create the posting list for an entity, attribute combination.
		pl, decr := posting.GetOrCreate(elem.key, args.gid)
		defer decr()

		// If a posting list contains a value, we store that or else we store a nil
		// byte so that processing is consistent later.
		val, err := pl.Value()
		isValueEdge := err == nil
		newValue := &task.Value{ValType: int32(val.Tid)}
		if err == nil {
			newValue.Val = val.Value.([]byte)
		} else {
			newValue.Val = x.Nilbyte
		}
		result.Value = newValue

		// get facets.
		if q.FacetParam != nil {
			if isValueEdge {
				fs, err := pl.Facets(q.FacetParam)
				if err != nil {
					//return nil, err
					// TODO(kg): should return error ?
					continue
				}
				result.Facets = &facets.List{[]*facets.Facets{&facets.Facets{fs}}}
			} else {
				result.Facets = &facets.List{pl.FacetsForUids(opts, q.FacetParam)}
			}
		}

		result.List = &emptyUIDList
		if q.DoCount {
			result.Count = uint32(pl.Length(0))
		} else if fnType == AggregatorFn {
		} else if fnType == PasswordFn {
			if len(newValue.Val) == 0 {
				result.Value = task.FalseVal
			}
			pwd := q.SrcFunc[1]
			err = types.VerifyPassword(pwd, string(newValue.Val))
			if err != nil {
				result.Value = task.FalseVal
			} else {
				result.Value = task.TrueVal
			}
		} else if fnType == CompareScalarFn {
			count := int64(pl.Length(0))
			if EvalCompare(fname, count, args.threshold) {
				result.List = algo.SortedListToBlock([]uint64{elem.uid})
			}
		} else {
			// The more usual case: Getting the UIDs.
			result.List = pl.Uids(opts)
		}
		chout <- result
	}
	close(chout)
}
