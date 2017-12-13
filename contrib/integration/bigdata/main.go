package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

/*

// keep track of where we're up to with xids
root
	[a-z]_start_xid - int
	[a-z]_end_xid - int

each node in the graph has an xid (string) in the form [a-z]_[0-9]+
there are 26 link predicates, link_[a-z]
there are 26 terminal predicates, attr_[a-z], these are just random strings

mod graph operation:

delete x nodes by taking a random [a-z] and deleting from the bottom of the [a-z]_start_xid
create x+y new nodes by choosing a random [a-z] and adding to the end of [a-z]_end_xid
Then set up some data.
	Each of the 52 terminal predicates gets a 10% chance of being present. Give each a random string.
	For each [a-z] link predicate, 80% chance it's not used, 10% chance it has 1 link, 5% chance it has 2 links, 5% chance it has 3 links. Just pick a random node to link to.


For querying:
	Pick a random xid, that's the start node.
	Then pick a depth to use, random between 1 and 4
	Expand all (manually...) on each level.


Correctness testing:
	- Should be able to mess around with nodes, bring them down up etc.
	- Should continually be able to ingest data. Maybe get the occasional transaction failure, but it should be a fairly constant rate.
	- Queries should continually succeed. They size should have a constant characteristic and follow a particular distribution.

*/

var setup = flag.Bool("setup", false, "sets up the initial schema and nodes")
var addrs = flag.String("addrs", "", "comma separated dgraph addresses")
var mode = flag.String("mode", "", "mode to run in ('mutate' or 'query')")
var conc = flag.Int("j", 1, "number of operations to run in parallel")

var (
	links     []string
	attrs     []string
	startXids []string
	endXids   []string
)

func init() {
	rand.Seed(time.Now().Unix())
	for i := 'a'; i <= 'z'; i++ {
		links = append(links, fmt.Sprintf("link_%c", i))
		attrs = append(attrs, fmt.Sprintf("attr_%c", i))
		startXids = append(startXids, fmt.Sprintf("start_xid_%c", i))
		endXids = append(endXids, fmt.Sprintf("end_xid_%c", i))
	}
}

func main() {
	flag.Parse()
	c := makeClient()

	if *setup {
		x.Check(c.Alter(context.Background(), &api.Operation{
			DropAll: true,
		}))
		x.Check(c.Alter(context.Background(), &api.Operation{
			Schema: schema(),
		}))
		x.Check2(c.NewTxn().Mutate(context.Background(), &api.Mutation{
			CommitNow: true,
			SetNquads: []byte(initialData()),
		}))

		// Add an initial node.
		r := &runner{txn: c.NewTxn(), ctx: context.Background()}
		_, xid, err := r.newNode()
		x.Check(err)
		x.Check(r.updateXidRanges(xid))
		x.Check(r.txn.Commit(r.ctx))
		return
	}

	switch *mode {
	case "mutate":
		var errCount int64
		var mutateCount int64
		for i := 0; i < *conc; i++ {
			go func() {
				for {
					err := doAdd(c)
					if err == nil {
						atomic.AddInt64(&mutateCount, 1)
					} else {
						atomic.AddInt64(&errCount, 1)
					}
				}
			}()
		}
		for {
			time.Sleep(time.Second)
			fmt.Printf("Status: success_mutations=%d errors=%d\n",
				atomic.LoadInt64(&mutateCount), atomic.LoadInt64(&errCount))
		}
	case "query":
		var errCount int64
		var queryCount int64
		for i := 0; i < *conc; i++ {
			go func() {
				for {
					err := expandGraph(c)
					if err == nil {
						atomic.AddInt64(&queryCount, 1)
					} else {
						fmt.Println(err)
						atomic.AddInt64(&errCount, 1)
					}
				}
			}()
		}
		for {
			time.Sleep(time.Second)
			fmt.Printf("Status: success_queries=%d errors=%d\n",
				atomic.LoadInt64(&queryCount), atomic.LoadInt64(&errCount))
		}
	default:
		fmt.Printf("unknown mode: %q\n", *mode)
		os.Exit(1)
	}
}

func schema() string {
	s := "xid: string @index(hash) .\n"
	for _, attr := range links {
		s += attr + ": uid @reverse .\n"
	}
	for _, attr := range attrs {
		s += attr + ": string .\n"
	}
	for _, attr := range append(startXids, endXids...) {
		s += attr + ": int .\n"
	}
	return s
}

func initialData() string {
	rdfs := "_:root <xid> \"root\" .\n"
	for _, attr := range startXids {
		rdfs += "_:root <" + attr + "> \"0\" .\n"
	}
	for _, attr := range endXids {
		rdfs += "_:root <" + attr + "> \"0\" .\n"
	}

	return rdfs
}

func makeClient() *client.Dgraph {
	var dgcs []api.DgraphClient
	for _, addr := range strings.Split(*addrs, ",") {
		c, err := grpc.Dial(addr, grpc.WithInsecure())
		x.Check(err)
		dgcs = append(dgcs, api.NewDgraphClient(c))
	}
	return client.NewDgraphClient(dgcs...)
}

type runner struct {
	ctx context.Context
	txn *client.Txn
}

func doAdd(c *client.Dgraph) error {
	r := &runner{
		ctx: context.Background(), // TODO
		txn: c.NewTxn(),
	}
	defer func() {
		r.txn.Discard(r.ctx)
	}()

	uid, xid, err := r.newNode()
	if err != nil {
		return err
	}

	rndUid, err := r.getRandomNodeUid()
	if err != nil {
		return err
	}
	char := 'a' + rune(rand.Intn(26))
	rdfs := fmt.Sprintf("<%s> <link_%c> <%s> .\n", uid, char, rndUid)

	for char := 'a'; char <= 'z'; char++ {
		if rand.Float64() < 0.9 {
			continue
		}
		payload := make([]byte, 16+rand.Intn(16))
		rand.Read(payload)
		rdfs += fmt.Sprintf("<%s> <attr_%c> \"%s\" .\n", uid, char, url.QueryEscape(string(payload)))
	}

	if _, err = r.txn.Mutate(context.Background(), &api.Mutation{SetNquads: []byte(rdfs)}); err != nil {
		return err
	}

	if err := r.updateXidRanges(xid); err != nil {
		return err
	}

	return r.txn.Commit(r.ctx)
}

func (r *runner) newNode() (string, string, error) {
	char := 'a' + rune(rand.Intn(26))
	var result struct {
		Q []struct {
			End *int
		}
	}
	if err := r.query(&result, `
	{
		q(func: eq(xid, "root")) {
			uid
			end: end_xid_%c
		}
	}`, char); err != nil {
		return "", "", err
	}
	x.AssertTrue(len(result.Q) == 1 && result.Q[0].End != nil)
	xid := fmt.Sprintf("%c_%d", char, *result.Q[0].End)
	assigned, err := r.txn.Mutate(context.Background(), &api.Mutation{
		SetNquads: []byte(fmt.Sprintf("_:node <xid> %q .\n", xid)),
	})
	if err != nil {
		return "", "", err
	}
	return assigned.Uids["node"], xid, nil
}

func (r *runner) updateXidRanges(newNodeXid string) error {
	char := rune(newNodeXid[0])
	x.AssertTrue(char >= 'a' && char <= 'z')

	var result struct {
		Q []struct {
			Uid *string
			End *int
		}
	}
	if err := r.query(&result, `
	{
		q(func: eq(xid, "root")) {
			uid
			end: end_xid_%c
		}
	}`, char); err != nil {
		return err
	}

	x.AssertTrue(len(result.Q) == 1 && result.Q[0].Uid != nil && result.Q[0].End != nil)

	_, err := r.txn.Mutate(r.ctx, &api.Mutation{
		SetNquads: []byte(fmt.Sprintf("<%s> <end_xid_%c> \"%d\" .\n",
			*result.Q[0].Uid, char, *result.Q[0].End+1)),
	})
	return err
}

func (r *runner) getRandomNodeUid() (string, error) {
	for {
		char := 'a' + rune(rand.Intn(26))
		var result struct {
			Q []struct {
				Start *int
				End   *int
			}
		}
		if err := r.query(&result, `
		{
			q(func: eq(xid, "root")) {
				start: start_xid_%c
				end: end_xid_%c
			}
		}`, char, char); err != nil {
			return "", err
		}

		x.AssertTrue(len(result.Q) == 1 && result.Q[0].Start != nil && result.Q[0].End != nil)
		var (
			start = *result.Q[0].Start
			end   = *result.Q[0].End
		)
		if start == end {
			continue // no nodes in this series
		}

		var res struct {
			Q []struct {
				Uid *string
			}
		}
		xid := fmt.Sprintf("%c_%d", char, rand.Intn(end-start)+start)
		if err := r.query(&res, `
		{
			q(func: eq(xid, "%s")) {
				uid
			}
		}
		`, xid); err != nil {
			return "", err
		}
		x.AssertTruef(len(res.Q) >= 1 && res.Q[0].Uid != nil, "len=%v", len(res.Q))
		return *res.Q[0].Uid, nil
	}
}

func (r *runner) query(out interface{}, q string, args ...interface{}) error {
	q = fmt.Sprintf(q, args...)
	resp, err := r.txn.Query(r.ctx, q)
	if err != nil {
		return err
	}
	return json.Unmarshal(resp.Json, out)
}

type AZ struct {
	A string
	B string
	C string
	D string
	E string
	F string
	G string
	H string
	I string
	J string
	K string
	L string
	M string
	N string
	O string
	P string
	Q string
	R string
	S string
	T string
	U string
	V string
	W string
	X string
	Y string
	Z string
}

func (a *AZ) fields() []string {
	var fs []string
	for _, f := range []string{
		a.A, a.B, a.C, a.D, a.E, a.F, a.G, a.H, a.I, a.J, a.K, a.L, a.M,
		a.N, a.O, a.P, a.Q, a.R, a.S, a.T, a.U, a.V, a.W, a.X, a.Y, a.Z,
	} {
		if f != "" {
			fs = append(fs, f)
		}
	}
	return fs
}

func showLeases(c *client.Dgraph) {
	resp, err := c.NewTxn().Query(context.Background(), `
	{
		q(func: eq(xid, "root")) {
			uid
			expand(_all_)
		}
	}
	`)
	x.Check(err)
	fmt.Println(prettyPrintJSON(resp.Json))
}

func prettyPrintJSON(j []byte) string {
	var m map[string]interface{}
	x.Check(json.Unmarshal(j, &m))
	pretty, err := json.MarshalIndent(m, "", "  ")
	x.Check(err)
	return string(pretty)
}

func expandGraph(c *client.Dgraph) error {
	r := runner{txn: c.NewTxn(), ctx: context.Background()}
	defer r.txn.Discard(r.ctx)

	uid, err := r.getRandomNodeUid()
	if err != nil {
		return err
	}

	q := ""
	for i := 0; i < 1+rand.Intn(3); i++ {
		q = "expand(_all_) { " + q + "}"
	}
	q = fmt.Sprintf(`
	{
		q(func: uid(%s)) {
			%s
		}
	}
	`, uid, q)

	resp, err := r.txn.Query(r.ctx, q)
	if err != nil {
		return err
	}

	_ = resp
	//fmt.Printf("Query:\n%s\n", q)
	//fmt.Printf("Response:\n%s\n", prettyPrintJSON(resp.Json))

	return nil
}
