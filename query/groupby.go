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
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/types"
)

type groupPair struct {
	val  types.Val
	attr string
}

type groups struct {
	group []GroupInfo
}

type GroupInfo struct {
	values     []groupPair
	aggregates []groupPair
	uids       []uint64
}

type groupElements struct {
	entities *taskp.List
	val      types.Val
}

type dedup struct {
	mp    []map[string]groupElements
	attrs []string
}

func (d *dedup) addValue(attr string, value types.Val, uid uint64) {
	idx := -1
	// Going last to first as we'll always add new ones to last and
	// would access it till we add a new entry.
	for i := len(d.attrs) - 1; i >= 0; i-- {
		it := d.attrs[i]
		if attr == it {
			idx = i
			break
		}
	}
	if idx == -1 {
		// Create a new entry.
		d.attrs = append(d.attrs, attr)
		d.mp = append(d.mp, make(map[string]groupElements))
		idx = len(d.attrs) - 1
	}

	// Create the string key.
	var strKey string
	if value.Tid == types.UidID {
		strKey = strconv.FormatUint(uid, 10)
	} else {
		valC := types.Val{types.StringID, ""}
		err := types.Marshal(value, &valC)
		if err != nil {
			return
		}
		strKey = valC.Value.(string)
	}

	if _, ok := d.mp[idx][strKey]; !ok {
		d.mp[idx][strKey] = groupElements{
			val:      value,
			entities: &taskp.List{make([]uint64, 0)},
		}
	}

	cur := d.mp[idx][strKey].entities
	cur.Uids = append(cur.Uids, uid)
}

// formGroup creates all possible groups with the list of uids that belong to that
// group.
func (res *groups) formGroups(l int, dedupMap dedup, cur *taskp.List, groupVal []groupPair) {
	if l == len(dedupMap.attrs) {
		a := make([]uint64, len(cur.Uids))
		b := make([]groupPair, len(groupVal))
		copy(a, cur.Uids)
		copy(b, groupVal)
		res.group = append(res.group, GroupInfo{
			uids:   a,
			values: b,
		})
		return
	}

	for _, v := range dedupMap.mp[l] {
		temp := new(taskp.List)
		groupVal = append(groupVal, groupPair{
			val:  v.val,
			attr: dedupMap.attrs[l],
		})
		if l != 0 {
			algo.IntersectWith(cur, v.entities, temp)
		} else {
			temp.Uids = make([]uint64, len(v.entities.Uids))
			copy(temp.Uids, v.entities.Uids)
		}
		res.formGroups(l+1, dedupMap, temp, groupVal)
		groupVal = groupVal[:len(groupVal)-1]
	}
}

func processGroupBy(sg *SubGraph) error {
	mp := make(map[string]GroupInfo)
	_ = mp
	var dedupMap dedup
	idx := 0
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
		} else {
			// It's a value node.
			for i, v := range child.values {
				srcUid := child.SrcUIDs.Uids[i]
				val, err := convertTo(v)
				if err != nil {
					continue
				}
				dedupMap.addValue(child.Attr, val, srcUid)
			}
		}
		idx++
	}

	// Create all the groups here.
	res := new(groups)
	var groupVal []groupPair
	res.formGroups(0, dedupMap, &taskp.List{}, groupVal)

	fmt.Println(res)

	// Go over the groups and aggregate the values.
	for i := range res.group {
		grp := &res.group[i]
		for _, child := range sg.Children {
			if child.Params.ignoreResult {
				continue
			}
			// This is a aggregation node.
			if child.Params.DoCount {
				(*grp).aggregates = append((*grp).aggregates, groupPair{
					attr: "count",
					val: types.Val{
						Tid:   types.IntID,
						Value: int64(len(grp.uids)),
					},
				})
			} else if len(child.SrcFunc) > 0 && isAggregatorFn(child.SrcFunc[0]) {
				fieldName := fmt.Sprintf("%s(%s)", child.SrcFunc[0], child.Attr)
				finalVal, err := child.aggregateGroup(grp)
				if err != nil {
					continue
				}
				(*grp).aggregates = append((*grp).aggregates, groupPair{
					attr: fieldName,
					val:  finalVal,
				})
			}
		}
	}
	sort.Slice(res.group, func(i, j int) bool {
		var u1, u2 uint64
		for _, it := range res.group[i].uids {
			u1 += it
		}
		for _, it := range res.group[j].uids {
			u2 += it
		}
		if u1 == u2 {
			if l, err := types.Less(res.group[i].values[0].val, res.group[j].values[0].val); err != nil {
				return l
			} else if l, err = types.Less(res.group[i].aggregates[0].val, res.group[j].aggregates[0].val); err != nil {
				return l
			}
		}
		return u1 < u2
	})
	sg.GroupbyRes = res
	return nil
}

func (child *SubGraph) aggregateGroup(grp *GroupInfo) (types.Val, error) {
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
		v := child.values[idx]
		val, err := convertWithBestEffort(v, child.Attr)
		if err != nil {
			continue
		}
		ag.Apply(val)
	}
	return ag.Value()
}
