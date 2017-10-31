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
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/dgraph-io/dgraph/dgraph"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/x"
)

// This method should just build the request and proxy it to the Query method of dgraph.Server.
// It can then encode the response as appropriate before sending it back to the user.
func queryHandler(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != "POST" {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	defer r.Body.Close()
	q, err := ioutil.ReadAll(r.Body)
	req := protos.Request{
		Query: string(q),
	}
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	resp, err := (&dgraph.Server{}).Query(ctx, &req)
	if err != nil {
		x.SetStatusWithData(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	response := map[string]interface{}{}
	if err := json.Unmarshal(resp.Json, &response); err != nil {
		x.SetStatusWithData(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	addLatency, _ := strconv.ParseBool(r.URL.Query().Get("latency"))
	debug, _ := strconv.ParseBool(r.URL.Query().Get("debug"))
	addLatency = addLatency || debug

	if addLatency {
		e := query.Extensions{
			Latency: resp.Latency,
			Txn:     response["txn"].(*protos.TxnContext),
		}
		response["extensions"] = e
		delete(response, "txn")
	}

	fmt.Printf("Resp: %+v\n", response)
	if js, err := json.Marshal(response); err == nil {
		w.Write(js)
	} else {
		x.SetStatusWithData(w, x.Error, "Unable to marshal response")
	}
	// TODO - Verify schema, server latency is encoded as it was before.
	// Encode linRead and readts under extensions key.
}

func mutationHandler(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != "POST" {
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

	mu.CommitImmediately = true
	resp, err := (&dgraph.Server{}).Mutate(context.Background(), mu)
	if err != nil {
		x.SetStatusWithData(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	e := query.Extensions{
		Txn: resp.Context,
	}
	response := map[string]interface{}{}
	response["extensions"] = e
	mp := map[string]interface{}{}
	mp["code"] = x.Success
	mp["message"] = "Done"
	mp["uids"] = query.ConvertUidsToHex(resp.Uids)
	response["data"] = mp
	fmt.Println("response", response)

	if js, err := json.Marshal(response); err == nil {
		w.Write(js)
	} else {
		x.SetStatusWithData(w, x.Error, "Unable to marshal schema")
	}
}
