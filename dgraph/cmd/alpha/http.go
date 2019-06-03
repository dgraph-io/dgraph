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

package alpha

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	"github.com/golang/protobuf/jsonpb"

	"google.golang.org/grpc/metadata"
)

func allowed(method string) bool {
	return method == http.MethodPost || method == http.MethodPut
}

// Common functionality for these request handlers. Returns true if the request is completely
// handled here and nothing further needs to be done.
func commonHandler(w http.ResponseWriter, r *http.Request) bool {
	// Do these requests really need CORS headers? Doesn't seem like it, but they are probably
	// harmless aside from the extra size they add to each response.
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		return true
	} else if !allowed(r.Method) {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return true
	}

	return false
}

// Read request body, transparently decompressing if necessary. Return nil on error.
func readRequest(w http.ResponseWriter, r *http.Request) []byte {
	var in io.Reader = r.Body

	if enc := r.Header.Get("Content-Encoding"); enc != "" && enc != "identity" {
		if enc == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				x.SetStatus(w, x.Error, "Unable to create decompressor")
				return nil
			}
			defer gz.Close()
			in = gz
		} else {
			x.SetStatus(w, x.ErrorInvalidRequest, "Unsupported content encoding")
			return nil
		}
	}

	body, err := ioutil.ReadAll(in)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return nil
	}

	return body
}

// parseUint64 reads the value for given URL parameter from request and
// parses it into uint64, empty string is converted into zero value
func parseUint64(r *http.Request, name string) (uint64, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return 0, nil
	}

	uintVal, err := strconv.ParseUint(value, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("Error: %+v while parsing %s as uint64", err, name)
	}

	return uintVal, nil
}

// parseBool reads the value for given URL parameter from request and
// parses it into bool, empty string is converted into zero value
func parseBool(r *http.Request, name string) (bool, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return false, nil
	}

	boolval, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("Error: %+v while parsing %s as bool", err, name)
	}

	return boolval, nil
}

// parseDuration reads the value for given URL parameter from request and
// parses it into time.Duration, empty string is converted into zero value
func parseDuration(r *http.Request, name string) (time.Duration, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return 0, nil
	}

	durationValue, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("Error: %+v while parsing %s as time.Duration", err, name)
	}

	return durationValue, nil
}

// Write response body, transparently compressing if necessary.
func writeResponse(w http.ResponseWriter, r *http.Request, b []byte) (int, error) {
	var out io.Writer = w

	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gzw := gzip.NewWriter(w)
		defer gzw.Close()
		out = gzw
	}

	return out.Write(b)
}

// This method should just build the request and proxy it to the Query method of dgraph.Server.
// It can then encode the response as appropriate before sending it back to the user.
func queryHandler(w http.ResponseWriter, r *http.Request) {
	if commonHandler(w, r) {
		return
	}

	isDebugMode, err := parseBool(r, "debug")
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}
	queryTimeout, err := parseDuration(r, "timeout")
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}
	startTs, err := parseUint64(r, "startTs")
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	body := readRequest(w, r)
	if body == nil {
		return
	}

	var params struct {
		Query     string            `json:"query"`
		Variables map[string]string `json:"variables"`
	}
	contentType := r.Header.Get("Content-Type")
	switch strings.ToLower(contentType) {
	case "application/json":
		if err := json.Unmarshal(body, &params); err != nil {
			x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
			return
		}

	case "application/graphqlpm":
		params.Query = string(body)

	default:
		x.SetStatus(w, x.ErrorInvalidRequest, "Unsupported Content-Type")
		return
	}

	ctx := context.WithValue(context.Background(), query.DebugKey, isDebugMode)
	ctx = attachAccessJwt(ctx, r)

	if queryTimeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, queryTimeout)
		defer cancel()
	}

	req := api.Request{
		Vars:    params.Variables,
		Query:   params.Query,
		StartTs: startTs,
	}

	if req.StartTs == 0 {
		// If be is set, run this as a best-effort query.
		isBestEffort, err := parseBool(r, "be")
		if err != nil {
			x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
			return
		}
		if isBestEffort {
			req.BestEffort = true
			req.ReadOnly = true
		}

		// If ro is set, run this as a readonly query.
		isReadOnly, err := parseBool(r, "ro")
		if err != nil {
			x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
			return
		}
		if isReadOnly {
			req.ReadOnly = true
		}
	}

	// Core processing happens here.
	resp, err := (&edgraph.Server{}).Query(ctx, &req)
	if err != nil {
		x.SetStatusWithData(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	var out bytes.Buffer
	writeEntry := func(key string, js []byte) {
		out.WriteRune('"')
		out.WriteString(key)
		out.WriteRune('"')
		out.WriteRune(':')
		out.Write(js)
	}

	e := query.Extensions{
		Txn:     resp.Txn,
		Latency: resp.Latency,
	}
	js, err := json.Marshal(e)
	if err != nil {
		x.SetStatusWithData(w, x.Error, err.Error())
		return
	}

	out.WriteRune('{')
	writeEntry("data", resp.Json)
	out.WriteRune(',')
	writeEntry("extensions", js)
	out.WriteRune('}')

	writeResponse(w, r, out.Bytes())
}

func mutationHandler(w http.ResponseWriter, r *http.Request) {
	if commonHandler(w, r) {
		return
	}

	commitNow, err := parseBool(r, "commitNow")
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}
	startTs, err := parseUint64(r, "startTs")
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}
	body := readRequest(w, r)
	if body == nil {
		return
	}

	// start parsing the query
	parseStart := time.Now()

	var mu *api.Mutation
	contentType := r.Header.Get("Content-Type")
	switch strings.ToLower(contentType) {
	case "application/json":
		ms := make(map[string]*skipJSONUnmarshal)
		if err := json.Unmarshal(body, &ms); err != nil {
			x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
			return
		}

		mu = &api.Mutation{}
		if setJSON, ok := ms["set"]; ok && setJSON != nil {
			mu.SetJson = setJSON.bs
		}
		if delJSON, ok := ms["delete"]; ok && delJSON != nil {
			mu.DeleteJson = delJSON.bs
		}

	case "application/rdf":
		// Parse N-Quads.
		mu, err = gql.ParseMutation(string(body))
		if err != nil {
			x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
			return
		}

	default:
		x.SetStatus(w, x.ErrorInvalidRequest, "Unsupported Content-Type")
		return
	}

	// end of query parsing
	parseEnd := time.Now()

	mu.StartTs = startTs
	mu.CommitNow = commitNow

	ctx := attachAccessJwt(context.Background(), r)
	resp, err := (&edgraph.Server{}).Mutate(ctx, mu)
	if err != nil {
		x.SetStatusWithData(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	resp.Latency.ParsingNs = uint64(parseEnd.Sub(parseStart).Nanoseconds())
	e := query.Extensions{
		Txn:     resp.Context,
		Latency: resp.Latency,
	}
	sort.Strings(e.Txn.Keys)
	sort.Strings(e.Txn.Preds)

	// Don't send keys array which is part of txn context if its commit immediately.
	if mu.CommitNow {
		e.Txn.Keys = e.Txn.Keys[:0]
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

	_, _ = writeResponse(w, r, js)
}

func commitHandler(w http.ResponseWriter, r *http.Request) {
	if commonHandler(w, r) {
		return
	}

	startTs, err := parseUint64(r, "startTs")
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}
	if startTs == 0 {
		x.SetStatus(w, x.ErrorInvalidRequest,
			"startTs parameter is mandatory while trying to commit")
		return
	}

	abort, err := parseBool(r, "abort")
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	var response map[string]interface{}
	if abort {
		response, err = handleAbort(startTs)
	} else {
		// Keys are sent as an array in the body.
		reqText := readRequest(w, r)
		if reqText == nil {
			return
		}

		response, err = handleCommit(startTs, reqText)
	}
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	js, err := json.Marshal(response)
	if err != nil {
		x.SetStatusWithData(w, x.Error, err.Error())
		return
	}

	_, _ = writeResponse(w, r, js)
}

func handleAbort(startTs uint64) (map[string]interface{}, error) {
	tc := &api.TxnContext{
		StartTs: startTs,
		Aborted: true,
	}
	if _, err := worker.CommitOverNetwork(context.Background(), tc); err != nil {
		return nil, err
	}

	response := map[string]interface{}{}
	response["code"] = x.Success
	response["message"] = "Done"
	return response, nil
}

func handleCommit(startTs uint64, reqText []byte) (map[string]interface{}, error) {
	tc := &api.TxnContext{
		StartTs: startTs,
	}

	var reqList []string
	useList := false
	if err := json.Unmarshal(reqText, &reqList); err == nil {
		useList = true
	}

	var reqMap map[string][]string
	if err := json.Unmarshal(reqText, &reqMap); err != nil && !useList {
		return nil, err
	}

	if useList {
		tc.Keys = reqList
	} else {
		tc.Keys = reqMap["keys"]
		tc.Preds = reqMap["preds"]
	}

	cts, err := worker.CommitOverNetwork(context.Background(), tc)
	if err != nil {
		return nil, err
	}

	resp := &api.Assigned{}
	resp.Context = tc
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

	return response, nil
}

func attachAccessJwt(ctx context.Context, r *http.Request) context.Context {
	if accessJwt := r.Header.Get("X-Dgraph-AccessToken"); accessJwt != "" {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}

		md.Append("accessJwt", accessJwt)
		ctx = metadata.NewIncomingContext(ctx, md)
	}
	return ctx
}

func alterHandler(w http.ResponseWriter, r *http.Request) {
	if commonHandler(w, r) {
		return
	}

	b := readRequest(w, r)
	if b == nil {
		return
	}

	op := &api.Operation{}
	if err := jsonpb.UnmarshalString(string(b), op); err != nil {
		op.Schema = string(b)
	}

	glog.Infof("Got alter request via HTTP from %s\n", r.RemoteAddr)
	fwd := r.Header.Get("X-Forwarded-For")
	if len(fwd) > 0 {
		glog.Infof("The alter request is forwarded by %s\n", fwd)
	}

	md := metadata.New(nil)
	// Pass in an auth token, if present.
	md.Append("auth-token", r.Header.Get("X-Dgraph-AuthToken"))
	ctx := metadata.NewIncomingContext(context.Background(), md)
	ctx = attachAccessJwt(ctx, r)
	if _, err := (&edgraph.Server{}).Alter(ctx, op); err != nil {
		x.SetStatus(w, x.Error, err.Error())
		return
	}

	res := map[string]interface{}{}
	data := map[string]interface{}{}
	data["code"] = x.Success
	data["message"] = "Done"
	res["data"] = data

	js, err := json.Marshal(res)
	if err != nil {
		x.SetStatus(w, x.Error, err.Error())
		return
	}

	_, _ = writeResponse(w, r, js)
}

// skipJSONUnmarshal stores the raw bytes as is while JSON unmarshaling.
type skipJSONUnmarshal struct {
	bs []byte
}

func (sju *skipJSONUnmarshal) UnmarshalJSON(bs []byte) error {
	sju.bs = bs
	return nil
}
