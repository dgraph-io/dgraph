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

package main

import (
	"crypto/sha256"
	"fmt"

	"github.com/dgraph-io/dgraph/protos"
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
// func shareHandler(w http.ResponseWriter, r *http.Request) {
// 	var mr query.InternalMutation
// 	var err error
// 	var rawQuery []byte

// 	w.Header().Set("Content-Type", "application/json")
// 	x.AddCorsHeaders(w)
// 	if r.Method != "POST" {
// 		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
// 		return
// 	}
// 	ctx := context.Background()
// 	defer r.Body.Close()
// 	if rawQuery, err = ioutil.ReadAll(r.Body); err != nil || len(rawQuery) == 0 {
// 		if tr, ok := trace.FromContext(ctx); ok {
// 			tr.LazyPrintf("Error while reading the stringified query payload: %+v", err)
// 		}
// 		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request encountered.")
// 		return
// 	}

// 	fail := func() {
// 		if tr, ok := trace.FromContext(ctx); ok {
// 			tr.LazyPrintf("Error: %+v", err)
// 		}
// 		x.SetStatus(w, x.Error, err.Error())
// 	}
// 	nquads := gql.WrapNQ(NewSharedQueryNQuads(rawQuery), protos.DirectedEdge_SET)
// 	newUids, err := query.AssignUids(ctx, nquads)
// 	if err != nil {
// 		fail()
// 		return
// 	}
// 	if mr, err = query.ToInternal(ctx, nquads, nil, newUids); err != nil {
// 		fail()
// 		return
// 	}
// 	if err = query.ApplyMutations(ctx, &protos.Mutations{Edges: mr.Edges}); err != nil {
// 		fail()
// 		return
// 	}
// 	tempMap := query.StripBlankNode(newUids)
// 	allocIdsStr := query.ConvertUidsToHex(tempMap)
// 	payload := map[string]interface{}{
// 		"code":    x.Success,
// 		"message": "Done",
// 		"uids":    allocIdsStr,
// 	}

// 	if res, err := json.Marshal(payload); err == nil {
// 		w.Write(res)
// 	} else {
// 		x.SetStatus(w, "Error", "Unable to marshal map")
// 	}
// }
