package query

import (
	"fmt"
	"strings"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

func (sg *SubGraph) fieldName() string {
	fieldName := sg.Attr
	if sg.Params.Alias != "" {
		fieldName = sg.Params.Alias
	}
	return fieldName
}

func addCount(pc *SubGraph, count uint64, dst outputNode) {
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

func addInternalNode(pc *SubGraph, uid uint64, dst outputNode) error {
	sv, ok := pc.Params.uidToVal[uid]
	if !ok || sv.Value == nil {
		return nil
	}
	fieldName := aggWithVarFieldName(pc)
	dst.AddValue(fieldName, sv)
	return nil
}

func addCheckPwd(pc *SubGraph, vals []*pb.TaskValue, dst outputNode) {
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
	return fieldName + FacetDelimeter + f.Key
}

// This method gets the values and children for a subprotos.
func (sg *SubGraph) preTraverse(uid uint64, dst outputNode) error {
	if sg.Params.IgnoreReflex {
		if alreadySeen(sg.Params.parentIds, uid) {
			// A node can't have itself as the child at any level.
			return nil
		}
		// Push myself to stack before sending this to children.
		sg.Params.parentIds = append(sg.Params.parentIds, uid)
	}

	var invalidUids map[uint64]bool
	// We go through all predicate children of the subprotos.
	for _, pc := range sg.Children {
		if pc.Params.ignoreResult {
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
		if pc.Params.isGroupBy {
			if len(pc.GroupbyRes) <= idx {
				return errors.Errorf("Unexpected length while adding Groupby. Idx: [%v], len: [%v]",
					idx, len(pc.GroupbyRes))
			}
			dst.addGroupby(pc, pc.GroupbyRes[idx], pc.fieldName())
			continue
		}

		fieldName := pc.fieldName()
		if len(pc.counts) > 0 {
			addCount(pc, uint64(pc.counts[idx]), dst)

		} else if pc.SrcFunc != nil && pc.SrcFunc.Name == "checkpwd" {
			addCheckPwd(pc, pc.valueMatrix[idx].Values, dst)

		} else if idx < len(pc.uidMatrix) && len(pc.uidMatrix[idx].Uids) > 0 {
			var fcsList []*pb.Facets
			if pc.Params.Facet != nil {
				fcsList = pc.facetsMatrix[idx].FacetsList
			}

			if sg.Params.IgnoreReflex {
				pc.Params.parentIds = sg.Params.parentIds
			}
			// We create as many predicate entity children as the length of uids for
			// this predicate.
			ul := pc.uidMatrix[idx]
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

				if pc.Params.Facet != nil && len(fcsList) > childIdx {
					fs := fcsList[childIdx]
					for _, f := range fs.Facets {
						fVal, err := facets.ValFor(f)
						if err != nil {
							return err
						}

						uc.AddValue(facetName(fieldName, f), fVal)
					}
				}

				if !uc.IsEmpty() {
					if sg.Params.GetUid {
						uc.SetUID(childUID, "uid")
					}
					if pc.List {
						dst.AddListChild(fieldName, uc)
					} else {
						dst.AddMapChild(fieldName, uc, false)
					}
				}
			}
			if pc.Params.uidCount && !(pc.Params.uidCountAlias == "" && pc.Params.Normalize) {
				uc := dst.New(fieldName)
				c := types.ValueForType(types.IntID)
				c.Value = int64(len(ul.Uids))
				alias := pc.Params.uidCountAlias
				if alias == "" {
					alias = "count"
				}
				uc.AddValue(alias, c)
				dst.AddListChild(fieldName, uc)
			}
		} else {
			if pc.Params.Alias == "" && len(pc.Params.Langs) > 0 {
				fieldName += "@"
				fieldName += strings.Join(pc.Params.Langs, ":")
			}

			if pc.Attr == "uid" {
				dst.SetUID(uid, pc.fieldName())
				continue
			}

			if len(pc.facetsMatrix) > idx && len(pc.facetsMatrix[idx].FacetsList) > 0 {
				// in case of Value we have only one Facets
				for _, f := range pc.facetsMatrix[idx].FacetsList[0].Facets {
					fVal, err := facets.ValFor(f)
					if err != nil {
						return err
					}

					dst.AddValue(facetName(fieldName, f), fVal)
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

				if pc.Params.expandAll && len(pc.LangTags[idx].Lang) != 0 {
					if i >= len(pc.LangTags[idx].Lang) {
						return errors.Errorf(
							"pb.error: all lang tags should be either present or absent")
					}
					fieldNameWithTag := fieldName
					lang := pc.LangTags[idx].Lang[i]
					if lang != "" {
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

	if sg.Params.IgnoreReflex && len(sg.Params.parentIds) > 0 {
		// Lets pop the stack.
		sg.Params.parentIds = (sg.Params.parentIds)[:len(sg.Params.parentIds)-1]
	}

	// Only for shortest path query we wan't to return uid always if there is
	// nothing else at that level.
	if (sg.Params.GetUid && !dst.IsEmpty()) || sg.Params.shortest {
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
