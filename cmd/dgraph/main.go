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

	"golang.org/x/net/context"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
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
	noshare        = flag.Bool("noshare", false, "Don't allow sharing queries through the UI.")
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

type mutationResult struct {
	edges   []*taskp.DirectedEdge
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

func convertToNQuad(ctx context.Context, mutation string) ([]*graphp.NQuad, error) {
	var nquads []*graphp.NQuad
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

func convertToEdges(ctx context.Context, nquads []*graphp.NQuad) (mutationResult, error) {
	var edges []*taskp.DirectedEdge
	var mr mutationResult

	newUids := make(map[string]uint64)
	for _, nq := range nquads {
		if strings.HasPrefix(nq.Subject, "_:") {
			newUids[nq.Subject] = 0
		} else {
			// Only store xids that need to be marked as used.
			if _, err := strconv.ParseInt(nq.Subject, 0, 64); err != nil {
				uid, err := rdf.GetUid(nq.Subject)
				if err != nil {
					return mr, err
				}
				newUids[nq.Subject] = uid
			}
		}

		if len(nq.ObjectId) > 0 {
			if strings.HasPrefix(nq.ObjectId, "_:") {
				newUids[nq.ObjectId] = 0
			} else if !strings.HasPrefix(nq.ObjectId, "_uid_:") {
				uid, err := rdf.GetUid(nq.ObjectId)
				if err != nil {
					return mr, err
				}
				newUids[nq.ObjectId] = uid
			}
		}
	}

	if len(newUids) > 0 {
		if err := worker.AssignUidsOverNetwork(ctx, newUids); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while AssignUidsOverNetwork for newUids: %v", newUids))
			return mr, err
		}
	}

	// Wrapper for a pointer to graphp.Nquad
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

func applyMutations(ctx context.Context, m *taskp.Mutations) error {
	err := worker.MutateOverNetwork(ctx, m)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while MutateOverNetwork"))
		return err
	}
	return nil
}

const INTERNAL_SHARE = "_share_"

func ismutationAllowed(mutation *graphp.Mutation) error {
	if *nomutations {
		if *noshare {
			return x.Errorf("Mutations are forbidden on this server.")
		}

		// Sharing is allowed, lets check that mutation should have only internal
		// share predicate.
		if !hasOnlySharePred(mutation) {
			return x.Errorf("Only mutations with: %v as predicate are allowed ",
				INTERNAL_SHARE)
		}
	}
	// Mutations are allowed but sharing isn't allowed.
	if *noshare {
		if hasSharePred(mutation) {
			return x.Errorf("Mutations with: %v as predicate are not allowed ",
				INTERNAL_SHARE)
		}
	}
	return nil
}

func convertAndApply(ctx context.Context, mutation *graphp.Mutation) (map[string]uint64, error) {
	var allocIds map[string]uint64
	var m taskp.Mutations
	var err error
	var mr mutationResult

	if err := ismutationAllowed(mutation); err != nil {
		return nil, err
	}

	if mr, err = convertToEdges(ctx, mutation.Set); err != nil {
		return nil, err
	}
	m.Edges, allocIds = mr.edges, mr.newUids
	for i := range m.Edges {
		m.Edges[i].Op = taskp.DirectedEdge_SET
	}

	if mr, err = convertToEdges(ctx, mutation.Del); err != nil {
		return nil, err
	}
	for i := range mr.edges {
		edge := mr.edges[i]
		edge.Op = taskp.DirectedEdge_DEL
		m.Edges = append(m.Edges, edge)
	}

	m.Schema = mutation.Schema
	if err := applyMutations(ctx, &m); err != nil {
		return nil, x.Wrap(err)
	}
	return allocIds, nil
}

func enrichSchema(updates []*graphp.SchemaUpdate) error {
	for _, schema := range updates {
		typ := types.TypeID(schema.ValueType)
		if typ == types.UidID {
			continue
		}
		if len(schema.Tokenizer) == 0 && schema.Directive == graphp.SchemaUpdate_INDEX {
			schema.Tokenizer = []string{tok.Default(typ).Name()}
		} else if len(schema.Tokenizer) > 0 && schema.Directive != graphp.SchemaUpdate_INDEX {
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

// This function is used to run mutations for the requests from different
// language clients.
func runMutations(ctx context.Context, mu *graphp.Mutation) (map[string]uint64, error) {
	var allocIds map[string]uint64
	var err error

	if err = enrichSchema(mu.Schema); err != nil {
		return nil, err
	}

	mutation := &graphp.Mutation{Set: mu.Set, Del: mu.Del, Schema: mu.Schema}
	if allocIds, err = convertAndApply(ctx, mutation); err != nil {
		return nil, err
	}
	return allocIds, nil
}

// This function is used to run mutations for the requests received from the
// http client.
func mutationHandler(ctx context.Context, mu *gql.Mutation) (map[string]uint64, error) {
	var set []*graphp.NQuad
	var del []*graphp.NQuad
	var s []*graphp.SchemaUpdate
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

	mutation := &graphp.Mutation{Set: set, Del: del, Schema: s}
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
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	// Lets add the value of the debug query parameter to the context.
	ctx := context.WithValue(context.Background(), "debug", r.URL.Query().Get("debug"))

	if rand.Float64() < *tracing {
		tr := trace.New("Dgraph", "Query")
		defer tr.Finish()
		ctx = trace.NewContext(ctx, tr)
	}

	var l query.Latency
	l.Start = time.Now()
	defer r.Body.Close()
	req, err := ioutil.ReadAll(r.Body)
	q := string(req)
	if err != nil || len(q) == 0 {
		x.TraceError(ctx, x.Wrapf(err, "Error while reading query"))
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request encountered.")
		return
	}

	res, err := parseQueryAndMutation(ctx, gql.Request{
		Str:       q,
		Variables: map[string]string{},
		Http:      true,
	})
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

	var schemaNode []*graphp.SchemaNode
	if res.Schema != nil {
		if schemaNode, err = worker.GetSchemaOverNetwork(ctx, res.Schema); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while fetching schema"))
			x.SetStatus(w, x.Error, err.Error())
			return
		}
	}

	if len(res.Query) == 0 {
		mp := map[string]interface{}{}
		if res.Mutation != nil {
			mp["code"] = x.Success
			mp["message"] = "Done"
			mp["uids"] = allocIdsStr
		}
		// Either Schema or query can be specified
		if res.Schema != nil {
			mp["schema"] = schemaNode
		}
		if js, err := json.Marshal(mp); err == nil {
			w.Write(js)
		} else {
			x.SetStatus(w, "Error", "Unable to marshal map")
		}
		return
	}

	sgl, err := query.ProcessQuery(ctx, res, &l)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while Executing query"))
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

	w.Header().Set("Content-Type", "application/json")
	err = query.ToJson(&l, sgl, w, allocIdsStr)
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

func addGroupHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	groupId := r.URL.Query().Get("gid")
	if len(groupId) == 0 {
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request. No group id defined.")
		return
	}
	gid, err := strconv.ParseUint(groupId, 0, 32)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, "Not valid group id")
		return
	}
	nodeId := r.URL.Query().Get("nodeId")
	if len(nodeId) == 0 {
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request. No node id defined.")
		return
	}
	nid, err := strconv.ParseUint(nodeId, 0, 64)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, "Not valid node id")
		return
	}

	if err := worker.StartServingGroup(ctx, nid, uint32(gid)); err != nil {
		x.SetStatus(w, err.Error(), "AddGroup failed.")
	} else {
		x.SetStatus(w, x.Success, fmt.Sprint("Node %d started serving group %d",
			nid, gid))
	}
}

func removeGroupHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	groupId := r.URL.Query().Get("gid")
	if len(groupId) == 0 {
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request. No group id defined.")
		return
	}
	gid, err := strconv.ParseUint(groupId, 0, 32)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, "Not valid group id")
		return
	}
	nodeId := r.URL.Query().Get("nodeId")
	if len(nodeId) == 0 {
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request. No node id defined.")
		return
	}
	nid, err := strconv.ParseUint(nodeId, 0, 64)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, "Not valid node id")
		return
	}
	if err := worker.StopServingGroup(ctx, nid, uint32(gid)); err != nil {
		x.SetStatus(w, err.Error(), "RemoveGroup failed.")
	} else {
		x.SetStatus(w, x.Success, fmt.Sprint("Group %d belonging to node %d removed",
			gid, nid))
	}
}

func addServerHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	groupIds := r.URL.Query().Get("gid")
	var gids []uint32
	if len(groupIds) == 0 {
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request. No group ids defined.")
		return
	}
	for _, groupId := range strings.Split(groupIds, ",") {
		gid, err := strconv.ParseUint(groupId, 0, 32)
		if err != nil {
			x.SetStatus(w, x.ErrorInvalidRequest, "Not valid group ids")
			return
		}
		gids = append(gids, uint32(gid))
	}
	nodeId := r.URL.Query().Get("nodeId")
	if len(nodeId) == 0 {
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request. No node id defined.")
		return
	}
	nid, err := strconv.ParseUint(nodeId, 0, 64)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, "Not valid node id")
		return
	}

	if err := worker.AddServer(ctx, nid, gids); err != nil {
		x.SetStatus(w, err.Error(), "AddServer failed.")
	} else {
		x.SetStatus(w, x.Success, fmt.Sprint("Node %d started serving groups %v",
			nid, gids))
	}
}

func removeServerHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	nodeId := r.URL.Query().Get("nodeId")
	if len(nodeId) == 0 {
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request. No node id defined.")
		return
	}
	nid, err := strconv.ParseUint(nodeId, 0, 64)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, "Not valid node id")
		return
	}
	if err := worker.RemoveServer(ctx, nid); err != nil {
		x.SetStatus(w, err.Error(), "RemoveServer failed.")
	} else {
		x.SetStatus(w, x.Success, fmt.Sprint("Server %d removed", nid))
	}
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

func hasGraphOps(mu *graphp.Mutation) bool {
	return len(mu.Set) > 0 || len(mu.Del) > 0 || len(mu.Schema) > 0
}

// server is used to implement graphp.DgraphServer
type grpcServer struct{}

// This method is used to execute the query and return the response to the
// client as a protocol buffer message.
func (s *grpcServer) Run(ctx context.Context,
	req *graphp.Request) (resp *graphp.Response, err error) {
	// we need membership information
	if !worker.HealthCheck() {
		x.Trace(ctx, "This server hasn't yet been fully initiated. Please retry later.")
		return resp, x.Errorf("Uninitiated server. Please retry later")
	}
	var allocIds map[string]uint64
	var schemaNodes []*graphp.SchemaNode
	if rand.Float64() < *tracing {
		tr := trace.New("Dgraph", "GrpcQuery")
		defer tr.Finish()
		ctx = trace.NewContext(ctx, tr)
	}

	resp = new(graphp.Response)
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
		if schemaNodes, err = worker.GetSchemaOverNetwork(ctx, schema); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while fetching schema"))
			return resp, err
		}
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

	gl := new(graphp.Latency)
	gl.Parsing, gl.Processing, gl.Pb = l.Parsing.String(), l.Processing.String(),
		l.ProtocolBuffer.String()
	resp.L = gl
	return resp, err
}

func (s *grpcServer) CheckVersion(ctx context.Context, c *graphp.Check) (v *graphp.Version,
	err error) {
	// we need membership information
	if !worker.HealthCheck() {
		x.Trace(ctx, "This server hasn't yet been fully initiated. Please retry later.")
		return v, x.Errorf("Uninitiated server. Please retry later")
	}

	v = new(graphp.Version)
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

func setupListener(addr string, port int) (net.Listener, error) {
	laddr := fmt.Sprintf("%s:%d", addr, port)
	if !*tlsEnabled {
		return net.Listen("tcp", laddr)
	}

	tlsCfg, err := x.GenerateTLSConfig(x.TLSHelperConfig{
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

	return tls.Listen("tcp", laddr, tlsCfg)
}

func serveGRPC(l net.Listener) {
	defer func() { finishCh <- struct{}{} }()
	s := grpc.NewServer(grpc.CustomCodec(&query.Codec{}))
	graphp.RegisterDgraphServer(s, &grpcServer{})
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
	http.HandleFunc("/debug/store", storeStatsHandler)
	http.HandleFunc("/admin/shutdown", shutDownHandler)
	http.HandleFunc("/admin/backup", backupHandler)
	http.HandleFunc("/admin/addServer", addServerHandler)
	http.HandleFunc("/admin/removeServer", removeServerHandler)
	http.HandleFunc("/admin/addGroup", addGroupHandler)
	http.HandleFunc("/admin/removeGroup", removeGroupHandler)

	// UI related API's.
	// Share urls have a hex string as the shareId. So if
	// our url path matches it, we wan't to serve index.html.
	reg := regexp.MustCompile(`\/0[xX][0-9a-fA-F]+`)
	http.Handle("/", homeHandler(http.FileServer(http.Dir(uiDir)), reg))
	http.HandleFunc("/ui/keywords", keywordHandler)
	http.HandleFunc("/ui/init", initialState)

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
	ps, err := store.NewSyncStore(*postingDir)
	x.Checkf(err, "Error initializing postings store")
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
