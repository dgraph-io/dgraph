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
	"bytes"
	"fmt"
	"strconv"
	"sync"

	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

const numGo = 4

// rdfBuilder is used to generate RDF from subgraph.
type rdfBuilder struct {
	sync.Mutex
	buf  []byte
	sgCh chan *SubGraph
}

// ToRDF converts the given subgraph list into rdf format.
func ToRDF(l *Latency, sgl []*SubGraph) ([]byte, error) {
	var wg sync.WaitGroup
	b := &rdfBuilder{
		sgCh: make(chan *SubGraph, 16),
	}

	for i := 0; i < numGo; i++ {
		wg.Add(1)
		go b.worker(&wg)
	}

	for _, sg := range sgl {
		if err := validateSubGraphForRDF(sg); err != nil {
			return nil, err
		}
		for _, child := range sg.Children {
			if err := b.send(child); err != nil {
				return nil, err
			}
		}
	}
	// close the subgraph channel and wait for workers to finish
	close(b.sgCh)
	wg.Wait()

	return b.buf, nil
}

// send recursively validates the subgraph and sends the valid subgraphs to sgCh
func (b *rdfBuilder) send(sg *SubGraph) error {
	if sg.SrcUIDs != nil {
		if err := validateSubGraphForRDF(sg); err != nil {
			return err
		}
		b.sgCh <- sg
	}
	for _, child := range sg.Children {
		if err := b.send(child); err != nil {
			return err
		}
	}
	return nil
}

func (b *rdfBuilder) worker(wg *sync.WaitGroup) {
	defer wg.Done()
	for sg := range b.sgCh {
		b.rdfForSubgraph(sg)
	}
}

// rdfForSubgraph generates RDF and appends to the output parameter.
func (b *rdfBuilder) rdfForSubgraph(sg *SubGraph) {
	// handle the case of recurse queries
	// Do not generate RDF if all the children of sg null uidMatrix
	nonNullChild := false
	for _, ch := range sg.Children {
		if len(ch.uidMatrix) != 0 {
			nonNullChild = true
		}
	}

	if len(sg.Children) > 0 && !nonNullChild {
		return
	}

	buf := &bytes.Buffer{}
	for i, uid := range codec.GetUids(sg.SrcUIDs) {
		if sg.Params.IgnoreResult {
			// Skip ignored values.
			continue
		}
		if sg.IsInternal() {
			if sg.Params.Expand != "" {
				continue
			}
			// Check if we have val for the given uid. If you got uid then populate the rdf.
			val, ok := sg.Params.UidToVal[uid]
			if !ok && val.Value == nil {
				continue
			}
			outputval, err := getObjectVal(val)
			if err != nil {
				continue
			}
			writeRDF(buf, uid, []byte(sg.aggWithVarFieldName()), outputval)
			continue
		}
		switch {
		case len(sg.counts) > 0:
			// Add count rdf.
			rdfForCount(buf, uid, sg.counts[i], sg)
		case i < len(sg.uidMatrix) && codec.ListCardinality(sg.uidMatrix[i]) != 0 &&
			len(sg.Children) > 0:
			// Add posting list relation.
			rdfForUIDList(buf, uid, sg.uidMatrix[i], sg)
		case i < len(sg.valueMatrix):
			rdfForValueList(buf, uid, sg.valueMatrix[i], sg.fieldName())
		}
	}
	b.write(buf)
	return
}

func (b *rdfBuilder) write(buf *bytes.Buffer) {
	b.Lock()
	b.buf = append(b.buf, buf.Bytes()...)
	b.Unlock()
}

func writeRDF(buf *bytes.Buffer, subject uint64, predicate []byte, object []byte) {
	// add subject
	x.Check2(buf.Write(x.ToHex(subject, true)))
	x.Check(buf.WriteByte(' '))
	// add predicate
	writeTriple(buf, predicate)
	x.Check(buf.WriteByte(' '))
	// add object
	x.Check2(buf.Write(object))
	x.Check(buf.WriteByte(' '))
	x.Check(buf.WriteByte('.'))
	x.Check(buf.WriteByte('\n'))
}

func writeTriple(buf *bytes.Buffer, val []byte) {
	x.Check(buf.WriteByte('<'))
	x.Check2(buf.Write(val))
	x.Check(buf.WriteByte('>'))
}

// rdfForCount returns rdf for count fucntion.
func rdfForCount(buf *bytes.Buffer, subject uint64, count uint32, sg *SubGraph) {
	fieldName := sg.Params.Alias
	if fieldName == "" {
		fieldName = fmt.Sprintf("count(%s)", sg.Attr)
	}
	writeRDF(buf, subject, []byte(fieldName),
		quotedNumber([]byte(strconv.FormatUint(uint64(count), 10))))
}

// rdfForUIDList returns rdf for uid list.
func rdfForUIDList(buf *bytes.Buffer, subject uint64, list *pb.List, sg *SubGraph) {
	for _, destUID := range codec.GetUids(list) {
		if !sg.DestMap.Contains(destUID) {
			// This uid is filtered.
			continue
		}
		// Build object.
		writeRDF(buf, subject, []byte(sg.fieldName()), x.ToHex(destUID, true))
	}
}

// rdfForValueList returns rdf for the value list.
// Ignore RDF's for the attirbute `uid`.
func rdfForValueList(buf *bytes.Buffer, subject uint64, valueList *pb.ValueList,
	attr string) {
	for _, destValue := range valueList.Values {
		val, err := convertWithBestEffort(destValue, attr)
		if err != nil {
			continue
		}
		outputval, err := getObjectVal(val)
		if err != nil {
			continue
		}
		writeRDF(buf, subject, []byte(attr), outputval)
	}
}

func getObjectVal(v types.Val) ([]byte, error) {
	outputval, err := valToBytes(v)
	if err != nil {
		return nil, err
	}
	switch v.Tid {
	case types.UidID:
		return buildTriple(outputval), nil
	case types.IntID:
		return quotedNumber(outputval), nil
	case types.FloatID:
		return quotedNumber(outputval), nil
	case types.GeoID:
		return nil, errors.New("Geo id is not supported in rdf output")
	default:
		return outputval, nil
	}
}

func buildTriple(val []byte) []byte {
	buf := make([]byte, 0, 2+len(val))
	buf = append(buf, '<')
	buf = append(buf, val...)
	buf = append(buf, '>')
	return buf
}

func validateSubGraphForRDF(sg *SubGraph) error {
	if sg.IsGroupBy() {
		return errors.New("groupby is not supported in rdf output format")
	}
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
	if sg.Params.Facet != nil && !sg.Params.ExpandAll {
		return errors.New("facets are not supported in the rdf output format")
	}
	return nil
}

func quotedNumber(val []byte) []byte {
	tmpVal := make([]byte, 0, len(val)+2)
	tmpVal = append(tmpVal, '"')
	tmpVal = append(tmpVal, val...)
	tmpVal = append(tmpVal, '"')
	return tmpVal
}
