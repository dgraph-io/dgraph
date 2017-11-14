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

package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

// NewSharedQueryNQuads returns nquads with query and hash.
func NewSharedQueryNQuads(query []byte) []*protos.NQuad {
	val := func(s string) *protos.Value {
		return &protos.Value{&protos.Value_DefaultVal{s}}
	}
	qHash := fmt.Sprintf("%x", sha256.Sum256(query))
	return []*protos.NQuad{
		{Subject: "_:share", Predicate: "_share_", ObjectValue: val(string(query))},
		{Subject: "_:share", Predicate: "_share_hash_", ObjectValue: val(qHash)},
	}
}

// shareHandler allows to share a query between users.
func shareHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var rawQuery []byte

	w.Header().Set("Content-Type", "application/json")
	x.AddCorsHeaders(w)
	if r.Method != "POST" {
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}
	ctx := context.Background()
	defer r.Body.Close()
	if rawQuery, err = ioutil.ReadAll(r.Body); err != nil || len(rawQuery) == 0 {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while reading the stringified query payload: %+v", err)
		}
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request encountered.")
		return
	}

	mu := &protos.Mutation{
		Set:               NewSharedQueryNQuads(rawQuery),
		CommitImmediately: true,
	}
	resp, err := (&edgraph.Server{}).Mutate(context.Background(), mu)
	if err != nil {
		x.SetStatusWithData(w, x.ErrorInvalidRequest, err.Error())
		return
	}
	mp := map[string]interface{}{}
	mp["code"] = x.Success
	mp["message"] = "Done"
	mp["uids"] = resp.Uids

	js, err := json.Marshal(mp)
	if err != nil {
		x.SetStatusWithData(w, x.Error, err.Error())
		return
	}
	w.Write(js)
}
