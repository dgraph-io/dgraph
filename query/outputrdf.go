/*
 * Copyright 2017-2020 Dgraph Labs, Inc. and Contributors
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
	"fmt"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/pkg/errors"
)

// ToRDF converts the given subgraph list into rdf format.
func ToRDF(l *Latency, sgl []*SubGraph) ([]byte, error) {
	output := ""
	for _, sg := range sgl {
		// Skip parent graph. we don't want parent values.
		for _, child := range sg.Children {
			if err := castToRDF(&output, child); err != nil {
				return nil, err
			}
		}

	}
	return []byte(output), nil
}

// castToRDF converts the given subgraph to RDF and appends to the
// output string.
func castToRDF(output *string, sg *SubGraph) error {

	uidCount := sg.Attr == "uid" && sg.Params.DoCount && sg.IsInternal()
	if uidCount {
		return errors.New("uid count is not supported in the rdf output format")
	}

	if sg.Params.Normalize {
		return errors.New("normalize directive is not supported in the rdf output format")
	}

	if sg.Params.IgnoreReflex {
		return errors.New("ignorereflex directive is not supported in the rdf output format")
	}

	if sg.SrcFunc != nil && sg.SrcFunc.Name == "checkpwd" {
		return errors.New("chkpwd function is not supported in the rdf output format")
	}

	if sg.SrcUIDs != nil {
		// Get RDF for the given subgraph.
		if err := rdfForSubgraph(output, sg); err != nil {
			return err
		}
	}

	// Recursively cnvert RDF for the children graph.
	for _, child := range sg.Children {
		err := castToRDF(output, child)
		if err != nil {
			return err
		}
	}
	return nil
}

// rdfForSubgraph generates RDF and appends to the output parameter.
func rdfForSubgraph(output *string, sg *SubGraph) error {
	for i, uid := range sg.SrcUIDs.Uids {
		if sg.Params.IgnoreResult {
			// Skip ignored values.
			continue
		}

		if sg.IsInternal() {
			if sg.Params.Expand != "" {
				continue
			}
			// Check if we have val for the given uid. If you got uid then populate
			// the rdf.
			val, ok := sg.Params.UidToVal[uid]
			if !ok && val.Value == nil {
				continue
			}
			outputval, err := valToBytes(val)
			if err != nil {
				return err
			}
			*output += fmt.Sprintf("<%#x> <%s> %s\n", uid, sg.aggWithVarFieldName(),
				string(outputval))
			continue
		}

		switch {
		case len(sg.counts) > 0:
			// Add count rdf.
			rdfForCount(output, uid, sg.counts[i], sg)
		case i < len(sg.uidMatrix) && len(sg.uidMatrix[i].Uids) != 0:
			// Add posting list relation.
			rdfForUIDList(output, uid, sg.uidMatrix[i], sg)
		case i < len(sg.valueMatrix):
			// TODO: add facet.
			rdfForValueList(output, uid, sg.valueMatrix[i], sg.fieldName())
		}
	}
	return nil
}

// rdfForCount returns rdf for count fucntion.
func rdfForCount(output *string, subject uint64, count uint32, sg *SubGraph) {
	fieldName := sg.Params.Alias
	if fieldName == "" {
		fieldName = fmt.Sprintf("count(%s)", sg.Attr)
	}
	*output += fmt.Sprintf("<%#x> <%s> %d \n", subject, fieldName, count)
}

// rdfForUIDList returns rdf for uid list.
func rdfForUIDList(output *string, subject uint64, list *pb.List, sg *SubGraph) {
	for _, destUID := range list.Uids {
		if algo.IndexOf(sg.DestUIDs, destUID) < 0 {
			// This uid is filtered.
			continue
		}
		// TODO: do the right RDF format.
		*output += fmt.Sprintf("<%#x> <%s> <%#x> \n", subject, sg.fieldName(), destUID)
	}
}

// rdfForValueList returns rdf for the value list.
func rdfForValueList(output *string, subject uint64, valueList *pb.ValueList, attr string) error {
	if attr == "uid" {
		*output += fmt.Sprintf("<%#x> <%s> <%#x> \n", subject, attr, subject)
		return nil
	}
	for _, destValue := range valueList.Values {
		val, err := convertWithBestEffort(destValue, attr)
		if err != nil {
			return err
		}
		outputval, err := valToBytes(val)
		if err != nil {
			return err
		}
		switch val.Tid {
		case types.UidID:
			*output += fmt.Sprintf("<%#x> <%s> <%s> \n", subject, attr, string(outputval))
		default:
			*output += fmt.Sprintf("<%#x> <%s> %s \n", subject, attr, string(outputval))
		}
	}
	return nil
}
