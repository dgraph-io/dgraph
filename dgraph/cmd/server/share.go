/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
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

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/x"
)

// NewSharedQueryNQuads returns nquads with query and hash.
func NewSharedQueryNQuads(query []byte) []*api.NQuad {
	val := func(s string) *api.Value {
		return &api.Value{&api.Value_DefaultVal{s}}
	}
	qHash := fmt.Sprintf("%x", sha256.Sum256(query))
	return []*api.NQuad{
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

	mu := &api.Mutation{
		Set:       NewSharedQueryNQuads(rawQuery),
		CommitNow: true,
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
