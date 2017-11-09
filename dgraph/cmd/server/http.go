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
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

const (
	startTsHeader = "X-Dgraph-StartTs"
)

func allowed(method string) bool {
	return method == http.MethodPost || method == http.MethodPut
}

// This method should just build the request and proxy it to the Query method of dgraph.Server.
// It can then encode the response as appropriate before sending it back to the user.
func queryHandler(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		return
	}

	if !allowed(r.Method) {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	req := protos.Request{}
	startTs := r.Header.Get(startTsHeader)
	if startTs != "" {
		ts, err := strconv.ParseUint(startTs, 0, 64)
		if err != nil {
			x.SetStatus(w, x.ErrorInvalidRequest,
				"Error while parsing StartTs header as uint64")
			return
		}
		req.StartTs = ts
	}

	linRead := r.Header.Get("X-Dgraph-LinRead")
	if linRead != "" {
		lr := make(map[uint32]uint64)
		if err := json.Unmarshal([]byte(linRead), &lr); err != nil {
			x.SetStatus(w, x.ErrorInvalidRequest,
				"Error while unmarshalling LinRead header into map")
			return
		}
		req.LinRead = &protos.LinRead{
			Ids: lr,
		}
	}

	defer r.Body.Close()
	q, err := ioutil.ReadAll(r.Body)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}
	req.Query = string(q)

	d := r.URL.Query().Get("debug")
	ctx := context.WithValue(context.Background(), "debug", d)
	resp, err := (&edgraph.Server{}).Query(ctx, &req)
	if err != nil {
		x.SetStatusWithData(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	response := map[string]interface{}{}

	e := query.Extensions{
		Txn:     resp.Txn,
		Latency: resp.Latency,
	}
	response["extensions"] = e

	// User can either ask for schema or have a query.
	if len(resp.Schema) > 0 {
		sort.Slice(resp.Schema, func(i, j int) bool {
			return resp.Schema[i].Predicate < resp.Schema[j].Predicate
		})
		js, err := json.Marshal(resp.Schema)
		if err != nil {
			x.SetStatusWithData(w, x.Error, "Unable to marshal schema")
			return
		}
		mp := map[string]interface{}{}
		mp["schema"] = json.RawMessage(string(js))
		response["data"] = mp
	} else {
		response["data"] = json.RawMessage(string(resp.Json))
	}

	if js, err := json.Marshal(response); err == nil {
		w.Write(js)
	} else {
		x.SetStatusWithData(w, x.Error, "Unable to marshal response")
	}
}

func mutationHandler(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		return
	}

	if !allowed(r.Method) {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}
	defer r.Body.Close()
	m, err := ioutil.ReadAll(r.Body)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	mu, err := gql.ParseMutation(string(m))
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	// Maybe rename it so that default is CommitImmediately.
	commit := r.Header.Get("X-Dgraph-CommitNow")
	if commit != "" {
		c, err := strconv.ParseBool(commit)
		if err != nil {
			x.SetStatus(w, x.ErrorInvalidRequest,
				"Error while parsing Commit header as bool")
			return
		}
		mu.CommitImmediately = c
	}

	startTs := r.Header.Get(startTsHeader)
	if startTs != "" {
		ts, err := strconv.ParseUint(startTs, 0, 64)
		if err != nil {
			x.SetStatus(w, x.ErrorInvalidRequest,
				"Error while parsing StartTs header as uint64")
			return
		}
		mu.StartTs = ts
	}

	resp, err := (&edgraph.Server{}).Mutate(context.Background(), mu)
	if err != nil {
		x.SetStatusWithData(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	// TODO - Don't send keys array which is part of txn context if its commit immediately.
	e := query.Extensions{
		Txn: resp.Context,
	}
	response := map[string]interface{}{}
	response["extensions"] = e
	mp := map[string]interface{}{}
	mp["code"] = x.Success
	mp["message"] = "Done"
	mp["uids"] = resp.Uids
	response["data"] = mp

	js, err := json.Marshal(response)
	if err != nil {
		x.SetStatusWithData(w, x.Error, err.Error())
		return
	}
	w.Write(js)
}

func commitHandler(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		return
	}

	if !allowed(r.Method) {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	resp := &protos.Assigned{}
	tc := &protos.TxnContext{}
	resp.Context = tc

	// Maybe pass start-ts and keys in body?
	startTs := r.Header.Get(startTsHeader)
	if startTs == "" {
		x.SetStatus(w, x.ErrorInvalidRequest,
			"StartTs header is mandatory while trying to commit")
		return
	}
	ts, err := strconv.ParseUint(startTs, 0, 64)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest,
			"Error while parsing StartTs header as uint64")
		return
	}
	tc.StartTs = ts

	keys := r.Header.Get("X-Dgraph-Keys")
	if keys == "" {
		x.SetStatus(w, x.ErrorInvalidRequest,
			"Keys header is mandatory while trying to commit")
		return
	}

	var k []string
	if err := json.Unmarshal([]byte(keys), &k); err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest,
			"Error while unmarshalling keys header into array")
		return
	}
	tc.Keys = k

	cts, aerr := worker.CommitOverNetwork(context.Background(), tc)
	if aerr != nil {
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	resp.Context.CommitTs = cts

	e := query.Extensions{
		Txn: resp.Context,
	}
	e.Txn.Keys = e.Txn.Keys[:0]
	response := map[string]interface{}{}
	response["extensions"] = e
	mp := map[string]interface{}{}
	mp["code"] = x.Success
	mp["message"] = "Done"
	response["data"] = mp

	js, err := json.Marshal(response)
	if err != nil {
		x.SetStatusWithData(w, x.Error, err.Error())
		return
	}
	w.Write(js)
}

func abortHandler(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		return
	}

	if !allowed(r.Method) {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	resp := &protos.Assigned{}
	tc := &protos.TxnContext{}
	resp.Context = tc

	// Maybe pass start-ts and keys in body?
	startTs := r.Header.Get(startTsHeader)
	if startTs == "" {
		x.SetStatus(w, x.ErrorInvalidRequest,
			"StartTs header is mandatory while trying to abort")
		return
	}
	ts, err := strconv.ParseUint(startTs, 0, 64)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest,
			"Error while parsing StartTs header as uint64")
		return
	}
	tc.StartTs = ts
	tc.Aborted = true

	_, aerr := worker.CommitOverNetwork(context.Background(), tc)
	if aerr != nil {
		x.SetStatus(w, x.Error, err.Error())
		return
	}

	response := map[string]interface{}{}
	response["code"] = x.Success
	response["message"] = "Done"

	js, err := json.Marshal(response)
	if err != nil {
		x.SetStatusWithData(w, x.Error, err.Error())
		return
	}
	w.Write(js)
}

func alterHandler(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		return
	}

	if !allowed(r.Method) {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	op := &protos.Operation{}

	defer r.Body.Close()
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	err = json.Unmarshal(b, &op)
	if err != nil {
		op.Schema = string(b)
	}

	_, err = (&edgraph.Server{}).Alter(context.Background(), op)
	if err != nil {
		x.SetStatus(w, x.Error, err.Error())
		return
	}

	res := map[string]interface{}{}
	res["code"] = x.Success
	res["message"] = "Done"
	js, err := json.Marshal(res)
	if err != nil {
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	w.Write(js)
}
