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
	"bufio"
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path"
	"regexp"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dgraph-io/badger/badger"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/soheilhy/cmux"
)

var (
	gconf      = flag.String("group_conf", "", "group configuration file")
	postingDir = flag.String("p", "p", "Directory to store posting lists.")
	walDir     = flag.String("w", "w", "Directory to store raft write-ahead logs.")
	port       = flag.Int("port", 8080, "Port to run server on.")
	bindall    = flag.Bool("bindall", false,
		"Use 0.0.0.0 instead of localhost to bind to all addresses on local machine.")
	nomutations    = flag.Bool("nomutations", false, "Don't allow mutations on this server.")
	tracing        = flag.Float64("trace", 0.0, "The ratio of queries to trace.")
	cpuprofile     = flag.String("cpu", "", "write cpu profile to file")
	memprofile     = flag.String("mem", "", "write memory profile to file")
	dumpSubgraph   = flag.String("dumpsg", "", "Directory to save subgraph for testing, debugging")
	finishCh       = make(chan struct{}) // channel to wait for all pending reqs to finish.
	shutdownCh     = make(chan struct{}) // channel to signal shutdown.
	pendingQueries = make(chan struct{}, 10000*runtime.NumCPU())
	// TLS configurations
	tlsEnabled       = flag.Bool("tls.on", false, "Use TLS connections with clients.")
	tlsCert          = flag.String("tls.cert", "", "Certificate file path.")
	tlsKey           = flag.String("tls.cert_key", "", "Certificate key file path.")
	tlsKeyPass       = flag.String("tls.cert_key_passphrase", "", "Certificate key passphrase.")
	tlsClientAuth    = flag.String("tls.client_auth", "", "Enable TLS client authentication")
	tlsClientCACerts = flag.String("tls.ca_certs", "", "CA Certs file path.")
	tlsSystemCACerts = flag.Bool("tls.use_system_ca", false, "Include System CA into CA Certs.")
	tlsMinVersion    = flag.String("tls.min_version", "TLS11", "TLS min version.")
	tlsMaxVersion    = flag.String("tls.max_version", "TLS12", "TLS max version.")
)

var mutationNotAllowedErr = x.Errorf("Mutations are forbidden on this server.")

type mutationResult struct {
	edges   []*protos.DirectedEdge
	newUids map[string]uint64
}

func stopProfiling() {
	// Stop the CPU profiling that was initiated.
	if len(*cpuprofile) > 0 {
		pprof.StopCPUProfile()
	}

	// Write memory profile before exit.
	if len(*memprofile) > 0 {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Println(err)
		}
		pprof.WriteHeapProfile(f)
		f.Close()
	}
}

func addCorsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers",
		"Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token,"+
			"X-Auth-Token, Cache-Control, X-Requested-With")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Connection", "close")
}

func convertToNQuad(ctx context.Context, mutation string) ([]*protos.NQuad, error) {
	var nquads []*protos.NQuad
	r := strings.NewReader(mutation)
	reader := bufio.NewReader(r)
	x.Trace(ctx, "Converting to NQuad")

	var strBuf bytes.Buffer
	var err error
	for {
		err = x.ReadLine(reader, &strBuf)
		if err != nil {
			break
		}
		ln := strings.Trim(strBuf.String(), " \t")
		if len(ln) == 0 {
			continue
		}
		nq, err := rdf.Parse(ln)
		if err == rdf.ErrEmpty { // special case: comment/empty line
			continue
		} else if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while parsing RDF"))
			return nquads, err
		}
		nquads = append(nquads, &nq)
	}
	if err != io.EOF {
		return nquads, err
	}
	return nquads, nil
}

func convertToEdges(ctx context.Context, nquads []*protos.NQuad) (mutationResult, error) {
	var edges []*protos.DirectedEdge
	var mr mutationResult

	newUids := make(map[string]uint64)
	for _, nq := range nquads {
		if strings.HasPrefix(nq.Subject, "_:") {
			newUids[nq.Subject] = 0
		} else {
			// Only store xids that need to be generated.
			_, err := rdf.ParseUid(nq.Subject)
			if err == rdf.ErrInvalidUID {
				return mr, err
			} else if err != nil {
				newUids[nq.Subject] = 0
			}
		}

		if len(nq.ObjectId) > 0 {
			if strings.HasPrefix(nq.ObjectId, "_:") {
				newUids[nq.ObjectId] = 0
			} else {
				_, err := rdf.ParseUid(nq.ObjectId)
				if err == rdf.ErrInvalidUID {
					return mr, err
				} else if err != nil {
					newUids[nq.ObjectId] = 0
				}
			}
		}
	}

	if len(newUids) > 0 {
		if err := worker.AssignUidsOverNetwork(ctx, newUids); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while AssignUidsOverNetwork for newUids: %v", newUids))
			return mr, err
		}
	}

	// Wrapper for a pointer to protos.Nquad
	var wnq rdf.NQuad
	for _, nq := range nquads {
		// Get edges from nquad using newUids.
		wnq = rdf.NQuad{nq}
		edge, err := wnq.ToEdgeUsing(newUids)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while converting to edge: %v", nq))
			return mr, err
		}
		edges = append(edges, edge)
	}

	resultUids := make(map[string]uint64)
	// Strip out _: prefix from the blank node keys.
	for k, v := range newUids {
		if strings.HasPrefix(k, "_:") {
			resultUids[k[2:]] = v
		}
	}

	mr = mutationResult{
		edges:   edges,
		newUids: resultUids,
	}
	return mr, nil
}

func AddInternalEdge(ctx context.Context, m *protos.Mutations) error {
	newEdges := make([]*protos.DirectedEdge, 0, 2*len(m.Edges))
	for _, mu := range m.Edges {
		x.AssertTrue(mu.Op == protos.DirectedEdge_DEL || mu.Op == protos.DirectedEdge_SET)
		if mu.Op == protos.DirectedEdge_SET {
			edge := &protos.DirectedEdge{
				Op:     protos.DirectedEdge_SET,
				Entity: mu.GetEntity(),
				Attr:   "_predicate_",
				Value:  []byte(mu.GetAttr()),
			}
			newEdges = append(newEdges, mu)
			newEdges = append(newEdges, edge)
		} else if mu.Op == protos.DirectedEdge_DEL {
			if mu.Attr != x.DeleteAllPredicates {
				newEdges = append(newEdges, mu)
				if string(mu.GetValue()) == x.DeleteAllObjects {
					// Delete the given predicate from _predicate_.
					edge := &protos.DirectedEdge{
						Op:     protos.DirectedEdge_DEL,
						Entity: mu.GetEntity(),
						Attr:   "_predicate_",
						Value:  []byte(mu.GetAttr()),
					}
					newEdges = append(newEdges, edge)
				}
			} else {
				// Fetch all the predicates and replace them
				preds, err := query.GetNodePredicates(ctx, &protos.List{[]uint64{mu.GetEntity()}})
				if err != nil {
					return err
				}
				val := mu.GetValue()
				for _, pred := range preds {
					edge := &protos.DirectedEdge{
						Op:     protos.DirectedEdge_DEL,
						Entity: mu.GetEntity(),
						Attr:   string(pred.Val),
						Value:  val,
					}
					newEdges = append(newEdges, edge)
				}
				edge := &protos.DirectedEdge{
					Op:     protos.DirectedEdge_DEL,
					Entity: mu.GetEntity(),
					Attr:   "_predicate_",
					Value:  val,
				}
				// Delete all the _predicate_ values
				edge.Attr = "_predicate_"
				newEdges = append(newEdges, edge)
			}
		}
	}
	m.Edges = newEdges
	return nil
}

func applyMutations(ctx context.Context, m *protos.Mutations) error {
	err := AddInternalEdge(ctx, m)
	if err != nil {
		return x.Wrapf(err, "While adding internal edges")
	}
	err = worker.MutateOverNetwork(ctx, m)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while MutateOverNetwork"))
		return err
	}
	return nil
}

func ismutationAllowed(ctx context.Context, mutation *protos.Mutation) bool {
	if *nomutations {
		shareAllowed, ok := ctx.Value("_share_").(bool)
		if !ok || !shareAllowed {
			return false
		}
	}

	return true
}

func convertAndApply(ctx context.Context, mutation *protos.Mutation) (map[string]uint64, error) {
	var allocIds map[string]uint64
	var m protos.Mutations
	var err error
	var mr mutationResult

	if !ismutationAllowed(ctx, mutation) {
		return nil, mutationNotAllowedErr
	}

	if mr, err = convertToEdges(ctx, mutation.Set); err != nil {
		return nil, err
	}
	m.Edges, allocIds = mr.edges, mr.newUids
	for i := range m.Edges {
		m.Edges[i].Op = protos.DirectedEdge_SET
	}

	if mr, err = convertToEdges(ctx, mutation.Del); err != nil {
		return nil, err
	}
	for i := range mr.edges {
		edge := mr.edges[i]
		edge.Op = protos.DirectedEdge_DEL
		m.Edges = append(m.Edges, edge)
	}

	m.Schema = mutation.Schema
	if err := applyMutations(ctx, &m); err != nil {
		return nil, x.Wrap(err)
	}
	return allocIds, nil
}

func enrichSchema(updates []*protos.SchemaUpdate) error {
	for _, schema := range updates {
		typ := types.TypeID(schema.ValueType)

		if (typ == types.UidID || typ == types.DefaultID || typ == types.PasswordID) &&
			schema.Directive == protos.SchemaUpdate_INDEX {
			return x.Errorf("Indexing not allowed on predicate %s of type %s",
				schema.Predicate, typ.Name())
		}

		if typ == types.UidID {
			continue
		}

		if len(schema.Tokenizer) == 0 && schema.Directive == protos.SchemaUpdate_INDEX {
			schema.Tokenizer = []string{tok.Default(typ).Name()}
		} else if len(schema.Tokenizer) > 0 && schema.Directive != protos.SchemaUpdate_INDEX {
			return x.Errorf("Tokenizers present without indexing on attr %s", schema.Predicate)
		}
		// check for valid tokeniser types and duplicates
		var seen = make(map[string]bool)
		var seenSortableTok bool
		for _, t := range schema.Tokenizer {
			tokenizer, has := tok.GetTokenizer(t)
			if !has {
				return x.Errorf("Invalid tokenizer %s", t)
			}
			if tokenizer.Type() != typ {
				return x.Errorf("Tokenizer: %s isn't valid for predicate: %s of type: %s",
					tokenizer.Name(), schema.Predicate, typ.Name())
			}
			if _, ok := seen[tokenizer.Name()]; !ok {
				seen[tokenizer.Name()] = true
			} else {
				return x.Errorf("Duplicate tokenizers present for attr %s", schema.Predicate)
			}
			if tokenizer.IsSortable() {
				if seenSortableTok {
					return x.Errorf("More than one sortable index encountered for: %v",
						schema.Predicate)
				}
				seenSortableTok = true
			}
		}
	}
	return nil
}

func parseFacets(nquads []*protos.NQuad) error {
	var err error
	var facet *protos.Facet
	for _, nq := range nquads {
		if len(nq.Facets) == 0 {
			continue
		}
		for idx, f := range nq.Facets {
			if facet, err = facets.FacetFor(f.Key, f.Val); err != nil {
				return err
			}
			nq.Facets[idx] = facet
		}

	}
	return nil
}

// Go client sends facets as string k-v pairs. So they need to parsed and tokenized
// on the server.
func parseFacetsInMutation(mu *protos.Mutation) error {
	if err := parseFacets(mu.Set); err != nil {
		return err
	}
	if err := parseFacets(mu.Del); err != nil {
		return err
	}
	return nil
}

// This function is used to run mutations for the requests from different
// language clients.
func runMutations(ctx context.Context, mu *protos.Mutation) (map[string]uint64, error) {
	var allocIds map[string]uint64
	var err error

	if err = enrichSchema(mu.Schema); err != nil {
		return nil, err
	}

	if err = parseFacetsInMutation(mu); err != nil {
		return nil, err
	}
	mutation := &protos.Mutation{Set: mu.Set, Del: mu.Del, Schema: mu.Schema}
	if allocIds, err = convertAndApply(ctx, mutation); err != nil {
		return nil, err
	}
	return allocIds, nil
}

// This function is used to run mutations for the requests received from the
// http client.
func mutationHandler(ctx context.Context, mu *gql.Mutation) (map[string]uint64, error) {
	var set []*protos.NQuad
	var del []*protos.NQuad
	var s []*protos.SchemaUpdate
	var allocIds map[string]uint64
	var err error

	if len(mu.Set) > 0 {
		if set, err = convertToNQuad(ctx, mu.Set); err != nil {
			return nil, x.Wrap(err)
		}
	}

	if len(mu.Del) > 0 {
		if del, err = convertToNQuad(ctx, mu.Del); err != nil {
			return nil, x.Wrap(err)
		}
	}

	if len(mu.Schema) > 0 {
		if s, err = schema.Parse(mu.Schema); err != nil {
			return nil, x.Wrap(err)
		}
	}

	mutation := &protos.Mutation{Set: set, Del: del, Schema: s}
	if allocIds, err = convertAndApply(ctx, mutation); err != nil {
		return nil, err
	}
	return allocIds, nil
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	if worker.HealthCheck() {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

// parseQueryAndMutation handles the cases where the query parsing code can hang indefinitely.
// We allow 1 second for parsing the query; and then give up.
func parseQueryAndMutation(ctx context.Context, r gql.Request) (res gql.Result, err error) {
	x.Trace(ctx, "Query received: %v", r.Str)
	errc := make(chan error, 1)

	go func() {
		var err error
		res, err = gql.Parse(r)
		errc <- err
	}()

	child, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	select {
	case <-child.Done():
		return res, child.Err()
	case err := <-errc:
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while parsing query"))
			return res, err
		}
		x.Trace(ctx, "Query parsed")
	}
	return res, nil
}

type wrappedErr struct {
	Err     error
	Message string
}

func hasGQLOps(mu *gql.Mutation) bool {
	return len(mu.Set) > 0 || len(mu.Del) > 0 || len(mu.Schema) > 0
}

func queryHandler(w http.ResponseWriter, r *http.Request) {
	if !worker.HealthCheck() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	// Add a limit on how many pending queries can be run in the system.
	pendingQueries <- struct{}{}
	defer func() { <-pendingQueries }()

	addCorsHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	// Lets add the value of the debug query parameter to the context.
	ctx := context.WithValue(context.Background(), "debug", r.URL.Query().Get("debug"))

	if rand.Float64() < *tracing {
		tr := trace.New("Dgraph", "Query")
		tr.SetMaxEvents(1000)
		defer tr.Finish()
		ctx = trace.NewContext(ctx, tr)
	}

	var l query.Latency
	l.Start = time.Now()
	defer r.Body.Close()
	req, err := ioutil.ReadAll(r.Body)
	q := string(req)
	if err != nil || len(q) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		x.TraceError(ctx, x.Wrapf(err, "Error while reading query"))
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request encountered.")
		return
	}

	parseStart := time.Now()
	res, err := parseQueryAndMutation(ctx, gql.Request{
		Str:       q,
		Variables: map[string]string{},
		Http:      true,
	})
	l.Parsing += time.Since(parseStart)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// set timeout if schema mutation not present
	if res.Mutation == nil || len(res.Mutation.Schema) == 0 {
		// If schema mutation is not present
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Minute)
		defer cancel()
	}

	var allocIds map[string]uint64
	var allocIdsStr map[string]string
	// If we have mutations, run them first.
	if res.Mutation != nil && hasGQLOps(res.Mutation) {
		if allocIds, err = mutationHandler(ctx, res.Mutation); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			x.TraceError(ctx, x.Wrapf(err, "Error while handling mutations"))
			x.SetStatus(w, x.Error, err.Error())
			return
		}
		// convert the new UIDs to hex string.
		allocIdsStr = make(map[string]string)
		for k, v := range allocIds {
			allocIdsStr[k] = fmt.Sprintf("%#x", v)
		}
	}

	var schemaNode []*protos.SchemaNode
	if res.Schema != nil {
		execStart := time.Now()
		if schemaNode, err = worker.GetSchemaOverNetwork(ctx, res.Schema); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			x.TraceError(ctx, x.Wrapf(err, "Error while fetching schema"))
			x.SetStatus(w, x.Error, err.Error())
			return
		}
		l.Processing += time.Since(execStart)
	}

	w.Header().Set("Content-Type", "application/json")
	var addLatency bool
	// If there is an error parsing, then addLatency would remain false.
	addLatency, _ = strconv.ParseBool(r.URL.Query().Get("latency"))
	debug, _ := strconv.ParseBool(r.URL.Query().Get("debug"))
	addLatency = addLatency || debug
	if len(res.Query) == 0 {
		mp := map[string]interface{}{}
		if res.Mutation != nil {
			mp["code"] = x.Success
			mp["message"] = "Done"
			mp["uids"] = allocIdsStr
		}
		// Either Schema or query can be specified
		if res.Schema != nil {
			js, err := json.Marshal(schemaNode)
			if err != nil {
				x.SetStatus(w, "Error", "Unable to marshal schema")
			}
			mp["schema"] = json.RawMessage(string(js))
			if addLatency {
				mp["server_latency"] = l.ToMap()
			}
		}
		if js, err := json.Marshal(mp); err == nil {
			w.Write(js)
		} else {
			w.WriteHeader(http.StatusBadRequest)
			x.SetStatus(w, "Error", "Unable to marshal map")
		}
		return
	}

	sgl, err := query.ProcessQuery(ctx, res, &l)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while Executing query"))
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	if len(*dumpSubgraph) > 0 {
		for _, sg := range sgl {
			x.Checkf(os.MkdirAll(*dumpSubgraph, 0700), *dumpSubgraph)
			s := time.Now().Format("20060102.150405.000000.gob")
			filename := path.Join(*dumpSubgraph, s)
			f, err := os.Create(filename)
			x.Checkf(err, filename)
			enc := gob.NewEncoder(f)
			x.Check(enc.Encode(sg))
			x.Checkf(f.Close(), filename)
		}
	}

	err = query.ToJson(&l, sgl, w, allocIdsStr, addLatency)
	if err != nil {
		// since we performed w.Write in ToJson above,
		// calling WriteHeader with 500 code will be ignored.
		x.TraceError(ctx, x.Wrapf(err, "Error while converting to Json"))
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	x.Trace(ctx, "Latencies: Total: %v Parsing: %v Process: %v Json: %v",
		time.Since(l.Start), l.Parsing, l.Processing, l.Json)
}

func shareHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	addCorsHeaders(w)

	if r.Method != "POST" {
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	// Set context to allow mutation
	ctx := context.WithValue(context.Background(), "_share_", true)

	defer r.Body.Close()
	rawQuery, err := ioutil.ReadAll(r.Body)
	if err != nil || len(rawQuery) == 0 {
		x.TraceError(ctx, x.Wrapf(err, "Error while reading the stringified query payload"))
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request encountered.")
		return
	}

	// Generate mutation with query and hash
	queryHash := sha256.Sum256(rawQuery)
	mutation := gql.Mutation{
		Set: fmt.Sprintf("<_:share> <_share_> %q . \n <_:share> <_share_hash_> \"%x\" .", rawQuery, queryHash),
	}

	var allocIds map[string]uint64
	var allocIdsStr map[string]string
	if allocIds, err = mutationHandler(ctx, &mutation); err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while handling mutations"))
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	// convert the new UIDs to hex string.
	allocIdsStr = make(map[string]string)
	for k, v := range allocIds {
		allocIdsStr[k] = fmt.Sprintf("%#x", v)
	}

	payload := map[string]interface{}{
		"code":    x.Success,
		"message": "Done",
		"uids":    allocIdsStr,
	}

	if res, err := json.Marshal(payload); err == nil {
		w.Write(res)
	} else {
		x.SetStatus(w, "Error", "Unable to marshal map")
	}
}

// storeStatsHandler outputs some basic stats for data store.
func storeStatsHandler(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<pre>"))
	w.Write([]byte(worker.StoreStats()))
	w.Write([]byte("</pre>"))
}

// handlerInit does some standard checks. Returns false if something is wrong.
func handlerInit(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "GET" {
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return false
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(ip).IsLoopback() {
		x.SetStatus(w, x.ErrorUnauthorized, fmt.Sprintf("Request from IP: %v", ip))
		return false
	}
	return true
}

func shutDownHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r) {
		return
	}

	shutdownServer()
	x.SetStatus(w, x.Success, "Server is shutting down")
}

func shutdownServer() {
	x.Printf("Got clean exit request")
	stopProfiling()          // stop profiling
	shutdownCh <- struct{}{} // exit grpc and http servers.

	// wait for grpc and http servers to finish pending reqs and
	// then stop all nodes, internal grpc servers and sync all the marks
	go func() {
		defer func() { shutdownCh <- struct{}{} }()

		// wait for grpc, http and http2 servers to stop
		<-finishCh
		<-finishCh
		<-finishCh

		worker.BlockingStop()
	}()
}

func backupHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r) {
		return
	}
	ctx := context.Background()
	if err := worker.BackupOverNetwork(ctx); err != nil {
		x.SetStatus(w, err.Error(), "Backup failed.")
		return
	}
	x.SetStatus(w, x.Success, "Backup completed.")
}

func hasGraphOps(mu *protos.Mutation) bool {
	return len(mu.Set) > 0 || len(mu.Del) > 0 || len(mu.Schema) > 0
}

// server is used to implement protos.DgraphServer
type grpcServer struct{}

// This method is used to execute the query and return the response to the
// client as a protocol buffer message.
func (s *grpcServer) Run(ctx context.Context,
	req *protos.Request) (resp *protos.Response, err error) {
	// we need membership information
	if !worker.HealthCheck() {
		x.Trace(ctx, "This server hasn't yet been fully initiated. Please retry later.")
		return resp, x.Errorf("Uninitiated server. Please retry later")
	}
	var allocIds map[string]uint64
	var schemaNodes []*protos.SchemaNode
	if rand.Float64() < *tracing {
		tr := trace.New("Dgraph", "GrpcQuery")
		tr.SetMaxEvents(1000)
		defer tr.Finish()
		ctx = trace.NewContext(ctx, tr)
	}

	// Sanitize the context of the keys used for internal purposes only
	ctx = context.WithValue(ctx, "_share_", nil)

	resp = new(protos.Response)
	if len(req.Query) == 0 && req.Mutation == nil {
		x.TraceError(ctx, x.Errorf("Empty query and mutation."))
		return resp, fmt.Errorf("Empty query and mutation.")
	}

	var l query.Latency
	l.Start = time.Now()
	x.Trace(ctx, "Query received: %v, variables: %v", req.Query, req.Vars)
	res, err := parseQueryAndMutation(ctx, gql.Request{
		Str:       req.Query,
		Variables: req.Vars,
		Http:      false,
	})
	if err != nil {
		return resp, err
	}
	l.Parsing += time.Since(l.Start)

	// set timeout if schema mutation not present
	if res.Mutation == nil || len(res.Mutation.Schema) == 0 {
		// If schema mutation is not present
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Minute)
		defer cancel()
	}

	// If mutations are part of the query, we run them through the mutation handler
	// same as the http client.
	if res.Mutation != nil && hasGQLOps(res.Mutation) {
		if allocIds, err = mutationHandler(ctx, res.Mutation); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while handling mutations"))
			return resp, err
		}
	}

	// Mutations are sent as part of the mutation object
	if req.Mutation != nil && hasGraphOps(req.Mutation) {
		if allocIds, err = runMutations(ctx, req.Mutation); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while handling mutations"))
			return resp, err
		}
	}
	resp.AssignedUids = allocIds

	if req.Schema != nil && res.Schema != nil {
		return resp, x.Errorf("Multiple schema blocks found")
	}
	// Schema Block can be part of query string
	schema := res.Schema
	if schema == nil {
		schema = req.Schema
	}

	if schema != nil {
		execStart := time.Now()
		if schemaNodes, err = worker.GetSchemaOverNetwork(ctx, schema); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while fetching schema"))
			return resp, err
		}
		l.Processing += time.Since(execStart)
	}
	resp.Schema = schemaNodes

	sgl, err := query.ProcessQuery(ctx, res, &l)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while converting to ProtocolBuffer"))
		return resp, err
	}

	nodes, err := query.ToProtocolBuf(&l, sgl)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while converting to ProtocolBuffer"))
		return resp, err
	}
	resp.N = nodes

	gl := new(protos.Latency)
	gl.Parsing, gl.Processing, gl.Pb = l.Parsing.String(), l.Processing.String(),
		l.ProtocolBuffer.String()
	resp.L = gl
	return resp, err
}

func (s *grpcServer) CheckVersion(ctx context.Context, c *protos.Check) (v *protos.Version,
	err error) {
	// we need membership information
	if !worker.HealthCheck() {
		x.Trace(ctx, "This server hasn't yet been fully initiated. Please retry later.")
		return v, x.Errorf("Uninitiated server. Please retry later")
	}

	v = new(protos.Version)
	v.Tag = x.Version()
	return v, nil
}

var uiDir string

func init() {
	// uiDir can also be set through -ldflags while doing a release build. In that
	// case it points to usr/local/share/dgraph/assets where we store assets for
	// the user. In other cases, it should point to the build directory within the repository.
	flag.StringVar(&uiDir, "ui", uiDir, "Directory which contains assets for the user interface")
	if uiDir == "" {
		uiDir = os.Getenv("GOPATH") + "/src/github.com/dgraph-io/dgraph/dashboard/build"
	}
}

func checkFlagsAndInitDirs() {
	if len(*cpuprofile) > 0 {
		f, err := os.Create(*cpuprofile)
		x.Check(err)
		pprof.StartCPUProfile(f)
	}

	// Create parent directories for postings, uids and mutations
	x.Check(os.MkdirAll(*postingDir, 0700))
}

func setupListener(addr string, port int) (listener net.Listener, err error) {
	var reload func()
	laddr := fmt.Sprintf("%s:%d", addr, port)
	if !*tlsEnabled {
		listener, err = net.Listen("tcp", laddr)
	} else {
		var tlsCfg *tls.Config
		tlsCfg, reload, err = x.GenerateTLSConfig(x.TLSHelperConfig{
			ConfigType:             x.TLSServerConfig,
			CertRequired:           *tlsEnabled,
			Cert:                   *tlsCert,
			Key:                    *tlsKey,
			KeyPassphrase:          *tlsKeyPass,
			ClientAuth:             *tlsClientAuth,
			ClientCACerts:          *tlsClientCACerts,
			UseSystemClientCACerts: *tlsSystemCACerts,
			MinVersion:             *tlsMinVersion,
			MaxVersion:             *tlsMaxVersion,
		})
		if err != nil {
			return nil, err
		}
		listener, err = tls.Listen("tcp", laddr, tlsCfg)
	}
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGHUP)
		for range sigChan {
			log.Println("SIGHUP signal received")
			if reload != nil {
				reload()
				log.Println("TLS certificates and CAs reloaded")
			}
		}
	}()
	return listener, err
}

func serveGRPC(l net.Listener) {
	defer func() { finishCh <- struct{}{} }()
	s := grpc.NewServer(grpc.CustomCodec(&query.Codec{}))
	protos.RegisterDgraphServer(s, &grpcServer{})
	err := s.Serve(l)
	log.Printf("gRpc server stopped : %s", err.Error())
	s.GracefulStop()
}

func serveHTTP(l net.Listener) {
	defer func() { finishCh <- struct{}{} }()
	srv := &http.Server{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 600 * time.Second,
		IdleTimeout:  2 * time.Minute,
	}

	err := srv.Serve(l)
	log.Printf("Stopped taking more http(s) requests. Err: %s", err.Error())
	ctx, cancel := context.WithTimeout(context.Background(), 630*time.Second)
	defer cancel()
	err = srv.Shutdown(ctx)
	log.Printf("All http(s) requests finished.")
	if err != nil {
		log.Printf("Http(s) shutdown err: %v", err.Error())
	}
}

func setupServer(che chan error) {
	go worker.RunServer(*bindall) // For internal communication.

	laddr := "localhost"
	if *bindall {
		laddr = "0.0.0.0"
	}

	l, err := setupListener(laddr, *port)
	if err != nil {
		log.Fatal(err)
	}

	tcpm := cmux.New(l)
	httpl := tcpm.Match(cmux.HTTP1Fast())
	grpcl := tcpm.MatchWithWriters(
		cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
	http2 := tcpm.Match(cmux.HTTP2())

	http.HandleFunc("/health", healthCheck)
	http.HandleFunc("/query", queryHandler)
	http.HandleFunc("/share", shareHandler)
	http.HandleFunc("/debug/store", storeStatsHandler)
	http.HandleFunc("/admin/shutdown", shutDownHandler)
	http.HandleFunc("/admin/backup", backupHandler)

	// UI related API's.
	// Share urls have a hex string as the shareId. So if
	// our url path matches it, we wan't to serve index.html.
	reg := regexp.MustCompile(`\/0[xX][0-9a-fA-F]+`)
	http.Handle("/", homeHandler(http.FileServer(http.Dir(uiDir)), reg))
	http.HandleFunc("/ui/keywords", keywordHandler)

	// Initilize the servers.
	go serveGRPC(grpcl)
	go serveHTTP(httpl)
	go serveHTTP(http2)

	go func() {
		<-shutdownCh
		// Stops grpc/http servers; Already accepted connections are not closed.
		l.Close()
	}()

	log.Println("grpc server started.")
	log.Println("http server started.")
	log.Println("Server listening on port", *port)

	err = tcpm.Serve() // Start cmux serving. blocking call
	<-shutdownCh       // wait for shutdownServer to finish
	che <- err         // final close for main.
}

func main() {
	rand.Seed(time.Now().UnixNano())
	x.Init()
	checkFlagsAndInitDirs()

	// All the writes to posting store should be synchronous. We use batched writers
	// for posting lists, so the cost of sync writes is amortized.
	opt := badger.DefaultOptions
	opt.SyncWrites = true
	opt.Dir = *postingDir
	opt.ValueDir = *postingDir
	ps, err := badger.NewKV(&opt)
	x.Checkf(err, "Error while creating badger KV posting store")
	defer ps.Close()

	x.Check(group.ParseGroupConfig(*gconf))
	schema.Init(ps)

	// Posting will initialize index which requires schema. Hence, initialize
	// schema before calling posting.Init().
	posting.Init(ps)
	worker.Init(ps)

	// setup shutdown os signal handler
	sdCh := make(chan os.Signal, 1)
	defer close(sdCh)
	// sigint : Ctrl-C, sigquit : Ctrl-\ (backslash), sigterm : kill command.
	signal.Notify(sdCh, os.Interrupt, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	go func() {
		_, ok := <-sdCh
		if ok {
			shutdownServer()
		}
	}()

	// Setup external communication.
	che := make(chan error, 1)
	go setupServer(che)
	go worker.StartRaftNodes(*walDir)

	if err := <-che; !strings.Contains(err.Error(),
		"use of closed network connection") {
		log.Fatal(err)
	}
}
