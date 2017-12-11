package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/url"
	"strings"

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

var (
	links     []string
	attrs     []string
	startXids []string
	endXids   []string
)

const (
	addPerRound    = 10
	deletePerRound = 1
)

func init() {
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
	}

	runMutation(context.Background(), c)

}

func schema() string {
	s := "xid: string @index(hash) .\n"
	for _, attr := range links {
		s += attr + ": uid .\n"
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
	for char := 'a'; char <= 'z'; char++ {
		rdfs += fmt.Sprintf("_:%c <xid> \"%c_0\" .\n", char, char)
	}
	for _, attr := range startXids {
		rdfs += "_:root <" + attr + "> \"0\" .\n"
	}
	for _, attr := range endXids {
		rdfs += "_:root <" + attr + "> \"1\" .\n"
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

func runMutation(ctx context.Context, c *client.Dgraph) {
	txn := c.NewTxn()
	defer txn.Discard(ctx)
	for i := 0; i < addPerRound; i++ {
		if err := runAdd(txn); err != nil {
			fmt.Println("Error:", err)
			return
		}
	}
	for i := 0; i < deletePerRound; i++ {
		if err := runDelete(txn); err != nil {
			fmt.Println("Error:", err)
			return
		}
	}
	if err := txn.Commit(ctx); err != nil {
		fmt.Println("Error:", err)
	}
}

func runAdd(txn *client.Txn) error {
	uid, err := newNode(txn)
	if err != nil {
		return err
	}
	var rdfs string
	for char := 'a'; char <= 'z'; char++ {
		var links int
		switch rnd := rand.Float64(); {
		case rnd < 0.80:
		case rnd < 0.90:
			links = 1
		case rnd < 0.95:
			links = 2
		default:
			links = 3
		}
		for i := 0; i < links; i++ {
			rndUid, err := getRandomNodeUid(txn)
			if err != nil {
				return err
			}
			rdfs += fmt.Sprintf("<%s> <link_%c> <%s> .\n", uid, char, rndUid)
		}
	}

	for char := 'a'; char <= 'z'; char++ {
		if rand.Float64() < 0.9 {
			continue
		}
		payload := make([]byte, 16+rand.Intn(16))
		rand.Read(payload)
		rdfs += fmt.Sprintf("<%s> <attr_%c> \"%s\" .\n", uid, char, url.QueryEscape(string(payload)))
	}

	if rdfs != "" {
		_, err = txn.Mutate(context.Background(), &api.Mutation{SetNquads: []byte(rdfs)})
		return err
	}
	return nil
}

func runDelete(txn *client.Txn) error {
	return nil
}

func newNode(txn *client.Txn) (string, error) {
	char := 'a' + rune(rand.Intn(26))
	var result struct {
		Q []struct {
			Uid *string
			End *int
		}
	}
	if err := query(context.Background(), txn, &result, `
	{
		q(func: eq(xid, "root")) {
			uid
			end: end_xid_%c
		}
	}`, char); err != nil {
		return "", err
	}
	if len(result.Q) != 1 || result.Q[0].End == nil || result.Q[0].Uid == nil {
		return "", x.Errorf("bad result %v", result)
	}

	assigned, err := txn.Mutate(context.Background(), &api.Mutation{
		SetNquads: []byte(fmt.Sprintf(`
			<%s> <end_xid_%c> "%d" .
			_:node <xid> %q .
			`,
			*result.Q[0].Uid, char, *result.Q[0].End+1,
			fmt.Sprintf("%c_%d", char, *result.Q[0].End)),
		),
	})
	if err != nil {
		return "", err
	}
	return assigned.Uids["node"], nil
}

func getRandomNodeUid(txn *client.Txn) (string, error) {
	for {
		char := 'a' + rune(rand.Intn(26))
		var result struct {
			Q []struct {
				Start *int
				End   *int
			}
		}
		if err := query(context.Background(), txn, &result, `
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
		fmt.Println("xid:", xid)
		if err := query(context.Background(), txn, &res, `
		{
			q(func: eq(xid, "%s")) {
				uid
			}
		}
		`, xid); err != nil {
			return "", err
		}
		fmt.Printf("%+v\n", res)
		x.AssertTrue(len(res.Q) == 1 && res.Q[0].Uid != nil)
		return *res.Q[0].Uid, nil
	}
}

func query(ctx context.Context, txn *client.Txn, out interface{}, q string, args ...interface{}) error {
	resp, err := txn.Query(ctx, fmt.Sprintf(q, args...))
	if err != nil {
		return err
	}
	return json.Unmarshal(resp.Json, out)
}
