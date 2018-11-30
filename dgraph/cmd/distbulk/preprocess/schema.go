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

package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
    "strings"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	wk "github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type schemaStore struct {
	m map[string]*pb.SchemaUpdate
}

func readSchema(filename string) []*pb.SchemaUpdate {
	f, err := os.Open(filename)
	x.Check(err)
	defer f.Close()

	var r io.Reader = f
	if filepath.Ext(filename) == ".gz" {
		r, err = gzip.NewReader(f)
		x.Check(err)
	}

	buf, err := ioutil.ReadAll(r)
	x.Check(err)

	initialSchema, err := schema.Parse(string(buf))
	x.Check(err)
	return initialSchema
}

func newSchemaStore(initial []*pb.SchemaUpdate, storeXids bool) *schemaStore {
	s := &schemaStore{
		m: map[string]*pb.SchemaUpdate{
			"_predicate_": &pb.SchemaUpdate{
                ValueType: pb.Posting_STRING,
                List:      true,
            },
		},
	}
	if storeXids {
		s.m["xid"] = &pb.SchemaUpdate{
			ValueType: pb.Posting_STRING,
			Tokenizer: []string{"hash"},
		}
	}
	for _, sch := range initial {
		p := sch.Predicate
		sch.Predicate = "" // Predicate is stored in the (badger) key, so not needed in the value.
		if _, ok := s.m[p]; ok {
			x.Check(fmt.Errorf("predicate %q already exists in schema", p))
		}
		s.m[p] = sch
	}
	return s
}

func (s *schemaStore) validateType(de *pb.DirectedEdge) {
	sch, ok := s.m[de.Attr]
	if !ok {
        sch = &pb.SchemaUpdate{ValueType: de.ValueType}
        s.m[de.Attr] = sch
	}

	err := wk.ValidateAndConvert(de, sch)
	if err != nil {
		log.Fatalf("RDF doesn't match schema: %v", err)
	}
}

func toSchemaFileString(pred string, sch *pb.SchemaUpdate) string {
    var b strings.Builder
    fmt.Fprintf(&b, "%s: ", pred)
    typestring := strings.ToLower(pb.Posting_ValType_name[int32(sch.ValueType)])
    if sch.List {
        fmt.Fprintf(&b, "[%s] ", typestring)
    } else {
        fmt.Fprintf(&b, "%s ", typestring)
    }

    switch sch.Directive {
    case pb.SchemaUpdate_INDEX:
        fmt.Fprintf(&b, "@index(%s) ", strings.Join(sch.Tokenizer, ", "))
    case pb.SchemaUpdate_REVERSE:
        b.WriteString("@reverse ")
    }

    if sch.Count {
        b.WriteString("@count ")
    }
    if sch.Upsert {
        b.WriteString("@upsert ")
    }
    if sch.Lang {
        b.WriteString("@lang ")
    }

    b.WriteString(".\n")
    return b.String()
}
