/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package query

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type groupPair struct {
	key  types.Val
	attr string
}

type groupResult struct {
	keys       []groupPair
	aggregates []groupPair
	uids       []uint64
}

func (grp *groupResult) aggregateChild(child *SubGraph) error {
	if child.Params.DoCount {
		if child.Attr != "_uid_" {
			return x.Errorf("Only _uid_ predicate is allowed in count within groupby")
		}
		grp.aggregates = append(grp.aggregates, groupPair{
			attr: "count",
			key: types.Val{
				Tid:   types.IntID,
				Value: int64(len(grp.uids)),
			},
		})
		return nil
	}
	if len(child.SrcFunc) > 0 && isAggregatorFn(child.SrcFunc[0]) {
		fieldName := fmt.Sprintf("%s(%s)", child.SrcFunc[0], child.Attr)
		finalVal, err := aggregateGroup(grp, child)
		if err != nil {
			return err
		}
		grp.aggregates = append(grp.aggregates, groupPair{
			attr: fieldName,
			key:  finalVal,
		})
	}
	return nil
}

type groupResults struct {
	group []*groupResult
}

type groupElements struct {
	entities *protos.List
	key      types.Val
}

type uniq struct {
	elements map[string]groupElements
	attr     string
}

type dedup struct {
	groups []*uniq
}

func (d *dedup) getGroup(attr string) *uniq {
	var res *uniq
	// Looping last to first is better in this case.
	for i := len(d.groups) - 1; i >= 0; i-- {
		it := d.groups[i]
		if attr == it.attr {
			res = it
			break
		}
	}
	if res == nil {
		// Create a new entry.
		res = &uniq{
			attr:     attr,
			elements: make(map[string]groupElements),
		}
		d.groups = append(d.groups, res)
	}
	return res
}

func (d *dedup) addValue(attr string, value types.Val, uid uint64) {
	cur := d.getGroup(attr)
	// Create the string key.
	var strKey string
	if value.Tid == types.UidID {
		strKey = strconv.FormatUint(value.Value.(uint64), 10)
	} else {
		valC := types.Val{types.StringID, ""}
		err := types.Marshal(value, &valC)
		if err != nil {
			return
		}
		strKey = valC.Value.(string)
	}

	if _, ok := cur.elements[strKey]; !ok {
		// If this is the first element of the group.
		cur.elements[strKey] = groupElements{
			key:      value,
			entities: &protos.List{make([]uint64, 0)},
		}
	}
	curEntity := cur.elements[strKey].entities
	curEntity.Uids = append(curEntity.Uids, uid)
}

func aggregateGroup(grp *groupResult, child *SubGraph) (types.Val, error) {
	ag := aggregator{
		name: child.SrcFunc[0],
	}
	for _, uid := range grp.uids {
		idx := sort.Search(len(child.SrcUIDs.Uids), func(i int) bool {
			return child.SrcUIDs.Uids[i] >= uid
		})
		if idx == len(child.SrcUIDs.Uids) || child.SrcUIDs.Uids[idx] != uid {
			continue
		}
		v := child.valueMatrix[idx].Values[0]
		val, err := convertWithBestEffort(v, child.Attr)
		if err != nil {
			continue
		}
		ag.Apply(val)
	}
	return ag.Value()
}

// formGroup creates all possible groups with the list of uids that belong to that
// group.
func (res *groupResults) formGroups(dedupMap dedup, cur *protos.List, groupVal []groupPair) {
	l := len(groupVal)
	if len(dedupMap.groups) == 0 || (l != 0 && len(cur.Uids) == 0) {
		// This group is already empty or no group can be formed. So stop.
		return
	}

	if l == len(dedupMap.groups) {
		a := make([]uint64, len(cur.Uids))
		b := make([]groupPair, len(groupVal))
		copy(a, cur.Uids)
		copy(b, groupVal)
		res.group = append(res.group, &groupResult{
			uids: a,
			keys: b,
		})
		return
	}

	for _, v := range dedupMap.groups[l].elements {
		temp := new(protos.List)
		groupVal = append(groupVal, groupPair{
			key:  v.key,
			attr: dedupMap.groups[l].attr,
		})
		if l != 0 {
			algo.IntersectWith(cur, v.entities, temp)
		} else {
			temp.Uids = make([]uint64, len(v.entities.Uids))
			copy(temp.Uids, v.entities.Uids)
		}
		res.formGroups(dedupMap, temp, groupVal)
		groupVal = groupVal[:len(groupVal)-1]
	}
}

func (sg *SubGraph) processGroupBy(doneVars map[string]varValue, path []*SubGraph) error {
	mp := make(map[string]groupResult)
	_ = mp
	var dedupMap dedup
	var pathNode *SubGraph
	for _, child := range sg.Children {
		if !child.Params.ignoreResult {
			continue
		}
		if len(child.DestUIDs.Uids) != 0 {
			// It's a UID node.
			for i := 0; i < len(child.uidMatrix); i++ {
				srcUid := child.SrcUIDs.Uids[i]
				ul := child.uidMatrix[i]
				for _, uid := range ul.Uids {
					dedupMap.addValue(child.Attr, types.Val{Tid: types.UidID, Value: uid}, srcUid)
				}
			}
			pathNode = child
		} else {
			// It's a value node.
			for i, v := range child.valueMatrix {
				srcUid := child.SrcUIDs.Uids[i]
				val, err := convertTo(v.Values[0])
				if err != nil {
					continue
				}
				dedupMap.addValue(child.Attr, val, srcUid)
			}
		}
	}

	// Create all the groups here.
	res := new(groupResults)
	res.formGroups(dedupMap, &protos.List{}, []groupPair{})

	// Go over the groups and aggregate the values.
	for _, child := range sg.Children {
		if child.Params.ignoreResult {
			continue
		}
		// This is a aggregation node.
		for _, grp := range res.group {
			err := grp.aggregateChild(child)
			if err != nil && err != ErrEmptyVal {
				return err
			}
		}
		chVar := child.Params.Var
		if chVar != "" {
			tempMap := make(map[uint64]types.Val)
			for _, grp := range res.group {
				if len(grp.keys) == 0 {
					continue
				}
				if len(grp.keys) > 1 {
					return x.Errorf("Expected one UID for var in groupby but got: %d", len(grp.keys))
				}
				uidVal := grp.keys[0].key.Value
				uid, ok := uidVal.(uint64)
				if !ok {
					return x.Errorf("Vars can be assigned only when grouped by UID attribute")
				}
				tempMap[uid] = grp.aggregates[len(grp.aggregates)-1].key
			}
			doneVars[chVar] = varValue{
				Vals: tempMap,
				path: append(path, pathNode),
			}
		}
		child.Params.ignoreResult = true
	}
	// Sort to order the groups for determinism.
	sort.Slice(res.group, func(i, j int) bool {
		return groupLess(res.group[i], res.group[j])
	})
	sg.GroupbyRes = res
	return nil
}

func groupLess(a, b *groupResult) bool {
	if len(a.uids) < len(b.uids) {
		return true
	} else if len(a.uids) != len(b.uids) {
		return false
	}
	if len(a.keys) < len(b.keys) {
		return true
	} else if len(a.keys) != len(b.keys) {
		return false
	}
	if len(a.aggregates) < len(b.aggregates) {
		return true
	} else if len(a.aggregates) != len(b.aggregates) {
		return false
	}

	for i := range a.keys {
		l, err := types.Less(a.keys[i].key, b.keys[i].key)
		if err == nil {
			if l {
				return l
			}
			l, _ = types.Less(b.keys[i].key, a.keys[i].key)
			if l {
				return !l
			}
		}
	}

	for i := range a.aggregates {
		if l, err := types.Less(a.aggregates[i].key, b.aggregates[i].key); err == nil {
			if l {
				return l
			}
			l, _ = types.Less(b.aggregates[i].key, a.aggregates[i].key)
			if l {
				return !l
			}
		}
	}
	x.Fatalf("wrong groups")
	return false
}
