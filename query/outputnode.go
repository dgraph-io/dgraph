/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package query

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"

	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

// ToJson converts the list of subgraph into a JSON response by calling toFastJSON.
func ToJson(l *Latency, sgl []*SubGraph) ([]byte, error) {
	sgr := &SubGraph{}
	for _, sg := range sgl {
		if sg.Params.Alias == "var" || sg.Params.Alias == "shortest" {
			continue
		}
		if sg.Params.GetUid {
			sgr.Params.GetUid = true
		}
		sgr.Children = append(sgr.Children, sg)
	}
	return sgr.toFastJSON(l)
}

func makeScalarNode(attr string, isChild bool, val []byte, list bool) *fastJsonNode {
	return &fastJsonNode{
		attr:      attr,
		isChild:   isChild,
		scalarVal: val,
		list:      list,
	}
}

type fastJsonNode struct {
	attr      string
	order     int // relative ordering (for sorted results)
	isChild   bool
	scalarVal []byte
	attrs     []*fastJsonNode
	list      bool
}

func (fj *fastJsonNode) AddValue(attr string, v types.Val) {
	fj.AddListValue(attr, v, false)
}

func (fj *fastJsonNode) AddListValue(attr string, v types.Val, list bool) {
	if bs, err := valToBytes(v); err == nil {
		fj.attrs = append(fj.attrs, makeScalarNode(attr, false, bs, list))
	}
}

func (fj *fastJsonNode) AddMapChild(attr string, val *fastJsonNode, isRoot bool) {
	var childNode *fastJsonNode
	for _, c := range fj.attrs {
		if c.attr == attr {
			childNode = c
			break
		}
	}

	if childNode != nil {
		val.isChild = true
		val.attr = attr
		childNode.attrs = append(childNode.attrs, val.attrs...)
	} else {
		val.isChild = false
		val.attr = attr
		fj.attrs = append(fj.attrs, val)
	}
}

func (fj *fastJsonNode) AddListChild(attr string, child *fastJsonNode) {
	child.attr = attr
	child.isChild = true
	fj.attrs = append(fj.attrs, child)
}

func (fj *fastJsonNode) New(attr string) *fastJsonNode {
	return &fastJsonNode{attr: attr, isChild: false}
}

func (fj *fastJsonNode) SetUID(uid uint64, attr string) {
	// if we're in debug mode, uid may be added second time, skip this
	if attr == "uid" {
		for _, a := range fj.attrs {
			if a.attr == attr {
				return
			}
		}
	}
	fj.attrs = append(fj.attrs, makeScalarNode(attr, false, []byte(fmt.Sprintf("\"%#x\"", uid)),
		false))
}

func (fj *fastJsonNode) IsEmpty() bool {
	return len(fj.attrs) == 0
}

var (
	boolTrue    = []byte("true")
	boolFalse   = []byte("false")
	emptyString = []byte(`""`)
)

func valToBytes(v types.Val) ([]byte, error) {
	switch v.Tid {
	case types.StringID, types.DefaultID:
		return json.Marshal(v.Value)
	case types.BinaryID:
		return []byte(fmt.Sprintf("%q", v.Value)), nil
	case types.IntID:
		// In types.Convert(), we always convert to int64 for IntID type. fmt.Sprintf is slow
		// and hence we are using strconv.FormatInt() here. Since int64 and int are most common int
		// types we are using FormatInt for those.
		switch num := v.Value.(type) {
		case int64:
			return []byte(strconv.FormatInt(num, 10)), nil
		case int:
			return []byte(strconv.FormatInt(int64(num), 10)), nil
		default:
			return []byte(fmt.Sprintf("%d", v.Value)), nil
		}
	case types.FloatID:
		return []byte(fmt.Sprintf("%f", v.Value)), nil
	case types.BoolID:
		if v.Value.(bool) {
			return boolTrue, nil
		}
		return boolFalse, nil
	case types.DateTimeID:
		// Return empty string instead of zero-time value string - issue#3166
		t := v.Value.(time.Time)
		if t.IsZero() {
			return emptyString, nil
		}
		return t.MarshalJSON()
	case types.GeoID:
		return geojson.Marshal(v.Value.(geom.T))
	case types.UidID:
		return []byte(fmt.Sprintf("\"%#x\"", v.Value)), nil
	case types.PasswordID:
		return []byte(fmt.Sprintf("%q", v.Value.(string))), nil
	default:
		return nil, errors.New("Unsupported types.Val.Tid")
	}
}

type nodeSlice []*fastJsonNode

func (n nodeSlice) Len() int {
	return len(n)
}

func (n nodeSlice) Less(i, j int) bool {
	cmp := strings.Compare(n[i].attr, n[j].attr)
	if cmp == 0 {
		return n[i].order < n[j].order
	}
	return cmp < 0
}

func (n nodeSlice) Swap(i, j int) {
	n[i], n[j] = n[j], n[i]
}

func (fj *fastJsonNode) writeKey(out *bytes.Buffer) error {
	if _, err := out.WriteRune('"'); err != nil {
		return err
	}
	if _, err := out.WriteString(fj.attr); err != nil {
		return err
	}
	if _, err := out.WriteRune('"'); err != nil {
		return err
	}
	if _, err := out.WriteRune(':'); err != nil {
		return err
	}
	return nil
}

func (fj *fastJsonNode) attachFacets(fieldName string, isList bool,
	fList []*api.Facet, facetIdx int) error {

	for _, f := range fList {
		fName := facetName(fieldName, f)
		fVal, err := facets.ValFor(f)
		if err != nil {
			return err
		}

		if !isList {
			fj.AddValue(fName, fVal)
		} else {
			facetNode := &fastJsonNode{attr: fName}
			facetNode.AddValue(strconv.Itoa(facetIdx), fVal)
			fj.AddMapChild(fName, facetNode, false)
		}
	}

	return nil
}

func (fj *fastJsonNode) encode(out *bytes.Buffer) error {
	// set relative ordering
	for i, a := range fj.attrs {
		a.order = i
	}

	i := 0
	if i < len(fj.attrs) {
		if _, err := out.WriteRune('{'); err != nil {
			return err
		}
		cur := fj.attrs[i]
		i++
		cnt := 1
		last := false
		inArray := false
		for {
			var next *fastJsonNode
			if i < len(fj.attrs) {
				next = fj.attrs[i]
				i++
			} else {
				last = true
			}

			if !last {
				if cur.attr == next.attr {
					if cnt == 1 {
						if err := cur.writeKey(out); err != nil {
							return err
						}
						if _, err := out.WriteRune('['); err != nil {
							return err
						}
						inArray = true
					}
					if err := cur.encode(out); err != nil {
						return err
					}
					cnt++
				} else {
					if cnt == 1 {
						if err := cur.writeKey(out); err != nil {
							return err
						}
						if cur.isChild || cur.list {
							if _, err := out.WriteRune('['); err != nil {
								return err
							}
							inArray = true
						}
					}
					if err := cur.encode(out); err != nil {
						return err
					}
					if cnt != 1 || (cur.isChild || cur.list) {
						if _, err := out.WriteRune(']'); err != nil {
							return err
						}
						inArray = false
					}
					cnt = 1
				}
				if _, err := out.WriteRune(','); err != nil {
					return err
				}

				cur = next
			} else {
				if cnt == 1 {
					if err := cur.writeKey(out); err != nil {
						return err
					}
				}
				if (cur.isChild || cur.list) && !inArray {
					if _, err := out.WriteRune('['); err != nil {
						return err
					}
				}
				if err := cur.encode(out); err != nil {
					return err
				}
				if cnt != 1 || (cur.isChild || cur.list) {
					if _, err := out.WriteRune(']'); err != nil {
						return err
					}
				}
				break
			}
		}
		if _, err := out.WriteRune('}'); err != nil {
			return err
		}
	} else {
		if _, err := out.Write(fj.scalarVal); err != nil {
			return err
		}
	}

	return nil
}

func merge(parent [][]*fastJsonNode, child [][]*fastJsonNode) ([][]*fastJsonNode, error) {
	if len(parent) == 0 {
		return child, nil
	}

	// Here we merge two slices of maps.
	mergedList := make([][]*fastJsonNode, 0, len(parent)*len(child))
	cnt := 0
	for _, pa := range parent {
		for _, ca := range child {
			cnt += len(pa) + len(ca)
			if cnt > x.Config.NormalizeNodeLimit {
				return nil, errors.Errorf(
					"Couldn't evaluate @normalize directive - too many results")
			}
			list := make([]*fastJsonNode, 0, len(pa)+len(ca))
			list = append(list, pa...)
			list = append(list, ca...)
			mergedList = append(mergedList, list)
		}
	}
	return mergedList, nil
}

// normalize returns all attributes of fj and its children (if any).
func (fj *fastJsonNode) normalize() ([][]*fastJsonNode, error) {
	cnt := 0
	for _, a := range fj.attrs {
		// Here we are counting all non-scalar attributes of fj. If there are any such
		// attributes, we will flatten it, otherwise we will return all attributes.

		// When we call addMapChild it tries to find whether there is already an attribute
		// with attr field same as attribute argument of addMapChild. If it doesn't find any
		// such attribute, it creates an attribute with isChild = false. In those cases
		// sometimes cnt remains zero  and normalize returns attributes without flattening.
		// So we are using len(a.attrs) > 0 instead of a.isChild
		if len(a.attrs) > 0 {
			cnt++
		}
	}

	if cnt == 0 {
		// Recursion base case
		// There are no children, we can just return slice with fj.attrs map.
		return [][]*fastJsonNode{fj.attrs}, nil
	}

	parentSlice := make([][]*fastJsonNode, 0, 5)
	// If the parents has attrs, lets add them to the slice so that it can be
	// merged with children later.
	attrs := make([]*fastJsonNode, 0, len(fj.attrs)-cnt)
	for _, a := range fj.attrs {
		// Check comment at previous occurrence of len(a.attrs) > 0
		if len(a.attrs) == 0 {
			attrs = append(attrs, a)
		}
	}
	parentSlice = append(parentSlice, attrs)

	for ci := 0; ci < len(fj.attrs); {
		childNode := fj.attrs[ci]
		// Check comment at previous occurrence of len(a.attrs) > 0
		if len(childNode.attrs) == 0 {
			ci++
			continue
		}
		childSlice := make([][]*fastJsonNode, 0, 5)
		for ci < len(fj.attrs) && childNode.attr == fj.attrs[ci].attr {
			childSlice = append(childSlice, fj.attrs[ci].attrs)
			ci++
		}
		// Merging with parent.
		var err error
		parentSlice, err = merge(parentSlice, childSlice)
		if err != nil {
			return nil, err
		}
	}
	for i, slice := range parentSlice {
		sort.Sort(nodeSlice(slice))

		first := -1
		last := 0
		for i := range slice {
			if slice[i].attr == "uid" {
				if first == -1 {
					first = i
				}
				last = i
			}
		}
		if first != -1 && first != last {
			if first == 0 {
				parentSlice[i] = slice[last:]
			} else {
				parentSlice[i] = append(slice[:first], slice[last:]...)
			}
		}
	}

	return parentSlice, nil
}

func (fj *fastJsonNode) addGroupby(sg *SubGraph, res *groupResults, fname string) {
	// Don't add empty groupby
	if len(res.group) == 0 {
		return
	}
	g := fj.New(fname)
	for _, grp := range res.group {
		uc := g.New("@groupby")
		for _, it := range grp.keys {
			uc.AddValue(it.attr, it.key)
		}
		for _, it := range grp.aggregates {
			uc.AddValue(it.attr, it.key)
		}
		g.AddListChild("@groupby", uc)
	}
	fj.AddListChild(fname, g)
}

func (fj *fastJsonNode) addAggregations(sg *SubGraph) error {
	for _, child := range sg.Children {
		aggVal, ok := child.Params.UidToVal[0]
		if !ok {
			if len(child.Params.NeedsVar) == 0 {
				return errors.Errorf("Only aggregated variables allowed within empty block.")
			}
			// the aggregation didn't happen, most likely was called with unset vars.
			// See: query.go:fillVars
			aggVal = types.Val{Tid: types.FloatID, Value: float64(0)}
		}
		if child.Params.Normalize && child.Params.Alias == "" {
			continue
		}
		fieldName := aggWithVarFieldName(child)
		n1 := fj.New(fieldName)
		n1.AddValue(fieldName, aggVal)
		fj.AddListChild(sg.Params.Alias, n1)
	}
	if fj.IsEmpty() {
		fj.AddListChild(sg.Params.Alias, &fastJsonNode{})
	}
	return nil
}

func handleCountUIDNodes(sg *SubGraph, n *fastJsonNode, count int) bool {
	addedNewChild := false
	fieldName := sg.fieldName()
	for _, child := range sg.Children {
		uidCount := child.Attr == "uid" && child.Params.DoCount && child.IsInternal()
		normWithoutAlias := child.Params.Alias == "" && child.Params.Normalize
		if uidCount && !normWithoutAlias {
			addedNewChild = true

			c := types.ValueForType(types.IntID)
			c.Value = int64(count)

			field := child.Params.Alias
			if field == "" {
				field = "count"
			}

			fjChild := n.New(fieldName)
			fjChild.AddValue(field, c)
			n.AddListChild(fieldName, fjChild)
		}
	}

	return addedNewChild
}

func processNodeUids(fj *fastJsonNode, sg *SubGraph) error {
	var seedNode *fastJsonNode
	if sg.Params.IsEmpty {
		return fj.addAggregations(sg)
	}

	if sg.uidMatrix == nil {
		fj.AddListChild(sg.Params.Alias, &fastJsonNode{})
		return nil
	}

	hasChild := handleCountUIDNodes(sg, fj, len(sg.DestUIDs.Uids))
	if sg.Params.IsGroupBy {
		if len(sg.GroupbyRes) == 0 {
			return errors.Errorf("Expected GroupbyRes to have length > 0.")
		}
		fj.addGroupby(sg, sg.GroupbyRes[0], sg.Params.Alias)
		return nil
	}

	lenList := len(sg.uidMatrix[0].Uids)
	for i := 0; i < lenList; i++ {
		uid := sg.uidMatrix[0].Uids[i]
		if algo.IndexOf(sg.DestUIDs, uid) < 0 {
			// This UID was filtered. So Ignore it.
			continue
		}

		n1 := seedNode.New(sg.Params.Alias)
		if err := sg.preTraverse(uid, n1); err != nil {
			if err.Error() == "_INV_" {
				continue
			}
			return err
		}

		if n1.IsEmpty() {
			continue
		}

		hasChild = true
		if !sg.Params.Normalize {
			fj.AddListChild(sg.Params.Alias, n1)
			continue
		}

		// Lets normalize the response now.
		normalized, err := n1.normalize()
		if err != nil {
			return err
		}
		for _, c := range normalized {
			fj.AddListChild(sg.Params.Alias, &fastJsonNode{attrs: c})
		}
	}

	if !hasChild {
		// So that we return an empty key if the root didn't have any children.
		fj.AddListChild(sg.Params.Alias, &fastJsonNode{})
	}
	return nil
}

// Extensions represents the extra information appended to query results.
type Extensions struct {
	Latency *api.Latency    `json:"server_latency,omitempty"`
	Txn     *api.TxnContext `json:"txn,omitempty"`
	Metrics *api.Metrics    `json:"metrics,omitempty"`
}

func (sg *SubGraph) toFastJSON(l *Latency) ([]byte, error) {
	encodingStart := time.Now()
	defer func() {
		l.Json = time.Since(encodingStart)
	}()

	var seedNode *fastJsonNode
	var err error
	n := seedNode.New("_root_")
	for _, sg := range sg.Children {
		err = processNodeUids(n, sg)
		if err != nil {
			return nil, err
		}
	}

	// According to GraphQL spec response should only contain data, errors and extensions as top
	// level keys. Hence we send server_latency under extensions key.
	// https://facebook.github.io/graphql/#sec-Response-Format

	var bufw bytes.Buffer
	if len(n.attrs) == 0 {
		if _, err := bufw.WriteString(`{}`); err != nil {
			return nil, err
		}
	} else {
		if err := n.encode(&bufw); err != nil {
			return nil, err
		}
	}
	return bufw.Bytes(), nil
}

func (sg *SubGraph) fieldName() string {
	fieldName := sg.Attr
	if sg.Params.Alias != "" {
		fieldName = sg.Params.Alias
	}
	return fieldName
}

func addCount(pc *SubGraph, count uint64, dst *fastJsonNode) {
	if pc.Params.Normalize && pc.Params.Alias == "" {
		return
	}
	c := types.ValueForType(types.IntID)
	c.Value = int64(count)
	fieldName := pc.Params.Alias
	if fieldName == "" {
		fieldName = fmt.Sprintf("count(%s)", pc.Attr)
	}
	dst.AddValue(fieldName, c)
}

func aggWithVarFieldName(pc *SubGraph) string {
	if pc.Params.Alias != "" {
		return pc.Params.Alias
	}
	fieldName := fmt.Sprintf("val(%v)", pc.Params.Var)
	if len(pc.Params.NeedsVar) > 0 {
		fieldName = fmt.Sprintf("val(%v)", pc.Params.NeedsVar[0].Name)
		if pc.SrcFunc != nil {
			fieldName = fmt.Sprintf("%s(%v)", pc.SrcFunc.Name, fieldName)
		}
	}
	return fieldName
}

func addInternalNode(pc *SubGraph, uid uint64, dst *fastJsonNode) error {
	sv, ok := pc.Params.UidToVal[uid]
	if !ok || sv.Value == nil {
		return nil
	}
	fieldName := aggWithVarFieldName(pc)
	dst.AddValue(fieldName, sv)
	return nil
}

func addCheckPwd(pc *SubGraph, vals []*pb.TaskValue, dst *fastJsonNode) {
	c := types.ValueForType(types.BoolID)
	if len(vals) == 0 {
		c.Value = false
	} else {
		c.Value = task.ToBool(vals[0])
	}

	fieldName := pc.Params.Alias
	if fieldName == "" {
		fieldName = fmt.Sprintf("checkpwd(%s)", pc.Attr)
	}
	dst.AddValue(fieldName, c)
}

func alreadySeen(parentIds []uint64, uid uint64) bool {
	for _, id := range parentIds {
		if id == uid {
			return true
		}
	}
	return false
}

func facetName(fieldName string, f *api.Facet) string {
	if f.Alias != "" {
		return f.Alias
	}
	return fieldName + x.FacetDelimeter + f.Key
}

// This method gets the values and children for a subprotos.
func (sg *SubGraph) preTraverse(uid uint64, dst *fastJsonNode) error {
	if sg.Params.IgnoreReflex {
		if alreadySeen(sg.Params.ParentIds, uid) {
			// A node can't have itself as the child at any level.
			return nil
		}
		// Push myself to stack before sending this to children.
		sg.Params.ParentIds = append(sg.Params.ParentIds, uid)
	}

	var invalidUids map[uint64]bool
	// We go through all predicate children of the subprotos.
	for _, pc := range sg.Children {
		if pc.Params.IgnoreResult {
			continue
		}
		if pc.IsInternal() {
			if pc.Params.Expand != "" {
				continue
			}
			if pc.Params.Normalize && pc.Params.Alias == "" {
				continue
			}
			if err := addInternalNode(pc, uid, dst); err != nil {
				return err
			}
			continue
		}

		if len(pc.uidMatrix) == 0 {
			// Can happen in recurse query.
			continue
		}
		if len(pc.facetsMatrix) > 0 && len(pc.facetsMatrix) != len(pc.uidMatrix) {
			return errors.Errorf("Length of facetsMatrix and uidMatrix mismatch: %d vs %d",
				len(pc.facetsMatrix), len(pc.uidMatrix))
		}

		idx := algo.IndexOf(pc.SrcUIDs, uid)
		if idx < 0 {
			continue
		}
		if pc.Params.IsGroupBy {
			if len(pc.GroupbyRes) <= idx {
				return errors.Errorf("Unexpected length while adding Groupby. Idx: [%v], len: [%v]",
					idx, len(pc.GroupbyRes))
			}
			dst.addGroupby(pc, pc.GroupbyRes[idx], pc.fieldName())
			continue
		}

		fieldName := pc.fieldName()
		switch {
		case len(pc.counts) > 0:
			addCount(pc, uint64(pc.counts[idx]), dst)

		case pc.SrcFunc != nil && pc.SrcFunc.Name == "checkpwd":
			addCheckPwd(pc, pc.valueMatrix[idx].Values, dst)

		case idx < len(pc.uidMatrix) && len(pc.uidMatrix[idx].Uids) > 0:
			var fcsList []*pb.Facets
			if pc.Params.Facet != nil {
				fcsList = pc.facetsMatrix[idx].FacetsList
			}

			if sg.Params.IgnoreReflex {
				pc.Params.ParentIds = sg.Params.ParentIds
			}

			// We create as many predicate entity children as the length of uids for
			// this predicate.
			ul := pc.uidMatrix[idx]
			// noneEmptyUID will store indexes of non empty UIDs.
			// This will be used for indexing facet response
			var nonEmptyUID []int
			for childIdx, childUID := range ul.Uids {
				if fieldName == "" || (invalidUids != nil && invalidUids[childUID]) {
					continue
				}
				uc := dst.New(fieldName)
				if rerr := pc.preTraverse(childUID, uc); rerr != nil {
					if rerr.Error() == "_INV_" {
						if invalidUids == nil {
							invalidUids = make(map[uint64]bool)
						}

						invalidUids[childUID] = true
						continue // next UID.
					}
					// Some other error.
					glog.Errorf("Error while traversal: %v", rerr)
					return rerr
				}

				if !uc.IsEmpty() {
					if sg.Params.GetUid {
						uc.SetUID(childUID, "uid")
					}
					nonEmptyUID = append(nonEmptyUID, childIdx) // append index to nonEmptyUID.

					if pc.Params.Normalize {
						// We will normalize at each level instead of
						// calling normalize after pretraverse.
						// Now normalize() only flattens one level,
						// the expectation is that its children have
						// already been normalized.
						normAttrs, err := uc.normalize()
						if err != nil {
							return err
						}

						for _, c := range normAttrs {
							// Adding as list child irrespective of the type of pc
							// (list or non-list), otherwise result might be inconsistent or might
							// depend on children and grandchildren of pc. Consider the case:
							// 	boss: uid .
							// 	friend: [uid] .
							// 	name: string .
							// For query like:
							// {
							// 	me(func: uid(0x1)) {
							// 		boss @normalize {
							// 			name
							// 		}
							// 	}
							// }
							// boss will be non list type in response, but for query like:
							// {
							// 	me(func: uid(0x1)) {
							// 		boss @normalize {
							// 			friend {
							// 				name
							// 			}
							// 		}
							// 	}
							// }
							// boss should be of list type because there can be mutliple friends of
							// boss.
							dst.AddListChild(fieldName, &fastJsonNode{attrs: c})
						}
						continue
					}
					if pc.List {
						dst.AddListChild(fieldName, uc)
					} else {
						dst.AddMapChild(fieldName, uc, false)
					}
				}
			}

			// Now fill facets for non empty UIDs.
			facetIdx := 0
			for _, uidIdx := range nonEmptyUID {
				if pc.Params.Facet != nil && len(fcsList) > uidIdx {
					fs := fcsList[uidIdx]
					err := dst.attachFacets(fieldName, pc.List, fs.Facets, facetIdx)
					if err != nil {
						return err
					}
					facetIdx++
				}
			}

			// add value for count(uid) nodes if any.
			_ = handleCountUIDNodes(pc, dst, len(ul.Uids))
		default:
			if pc.Params.Alias == "" && len(pc.Params.Langs) > 0 && pc.Params.Langs[0] != "*" {
				fieldName += "@"
				fieldName += strings.Join(pc.Params.Langs, ":")
			}

			if pc.Attr == "uid" {
				dst.SetUID(uid, pc.fieldName())
				continue
			}

			if len(pc.facetsMatrix) > idx && len(pc.facetsMatrix[idx].FacetsList) > 0 {
				// In case of Value we have only one Facets.
				for i, fcts := range pc.facetsMatrix[idx].FacetsList {
					if err := dst.attachFacets(fieldName, pc.List, fcts.Facets, i); err != nil {
						return err
					}
				}
			}

			if len(pc.valueMatrix) <= idx {
				continue
			}

			for i, tv := range pc.valueMatrix[idx].Values {
				// if conversion not possible, we ignore it in the result.
				sv, convErr := convertWithBestEffort(tv, pc.Attr)
				if convErr != nil {
					return convErr
				}

				if pc.Params.ExpandAll && len(pc.LangTags[idx].Lang) != 0 {
					if i >= len(pc.LangTags[idx].Lang) {
						return errors.Errorf(
							"pb.error: all lang tags should be either present or absent")
					}
					fieldNameWithTag := fieldName
					lang := pc.LangTags[idx].Lang[i]
					if lang != "" && lang != "*" {
						fieldNameWithTag += "@" + lang
					}
					encodeAsList := pc.List && len(lang) == 0
					dst.AddListValue(fieldNameWithTag, sv, encodeAsList)
					continue
				}

				encodeAsList := pc.List && len(pc.Params.Langs) == 0
				if !pc.Params.Normalize {
					dst.AddListValue(fieldName, sv, encodeAsList)
					continue
				}
				// If the query had the normalize directive, then we only add nodes
				// with an Alias.
				if pc.Params.Alias != "" {
					dst.AddListValue(fieldName, sv, encodeAsList)
				}
			}
		}
	}

	if sg.Params.IgnoreReflex && len(sg.Params.ParentIds) > 0 {
		// Lets pop the stack.
		sg.Params.ParentIds = (sg.Params.ParentIds)[:len(sg.Params.ParentIds)-1]
	}

	// Only for shortest path query we wan't to return uid always if there is
	// nothing else at that level.
	if (sg.Params.GetUid && !dst.IsEmpty()) || sg.Params.Shortest {
		dst.SetUID(uid, "uid")
	}

	if sg.pathMeta != nil {
		totalWeight := types.Val{
			Tid:   types.FloatID,
			Value: sg.pathMeta.weight,
		}
		dst.AddValue("_weight_", totalWeight)
	}

	return nil
}
