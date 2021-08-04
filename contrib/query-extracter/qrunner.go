package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"time"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"google.golang.org/grpc"
)

var (
	isQuery = regexp.MustCompile(`Got a query: query:(.*)`)
	queryRe = regexp.MustCompile(`".*?"`)
	varRe   = regexp.MustCompile(`vars:<.*?>`)
	keyRe   = regexp.MustCompile(`key:"(.*?)"`)
	valRe   = regexp.MustCompile(`value:"(.*?)"`)
)

type SchemaEntry struct {
	Predicate string `json="predicate"`
	Type      string `json="type"`
}

type Schema struct {
	Schema []*SchemaEntry
}

func main() {
	filePath := flag.String("log-file", "", "Path of the log file")
	alphaSrc := flag.String("alpha-src", "", "GRPC endpoint of alpha")
	alphaTest := flag.String("alpha-test", "", "GRPC endpoint of master alpha")
	counts := flag.Bool("get-counts", false, "Get the counts of all nodes for each predicate on "+
		"the src-alpha")
	flag.Parse()

	conn, err := grpc.Dial(*alphaSrc, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("While dialing grpc: %v\n", err)
	}
	defer conn.Close()
	dc := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	if *counts {
		getCounts(dc)
		return
	}

	conn2, err := grpc.Dial(*alphaTest, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("While dialing grpc: %v\n", err)
	}
	defer conn2.Close()
	dcMaster := dgo.NewDgraphClient(api.NewDgraphClient(conn2))

	f, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("While opening log file got error: %v", err)
	}

	defer f.Close()
	failed := 0
	total := 0
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := scan.Text()
		r := getReq(line)
		if r != nil {
			total++
			resp := runQuery(r, dc)
			respMaster := runQuery(r, dcMaster)
			if !areEqualJSON(resp, respMaster) {
				failed++
				fmt.Printf("Failed on Query: %s vars: %+v\n", r.Query, r.Vars)
				fmt.Printf("Response cloud: %s\n\n Master: %s \n\n", resp, respMaster)
			}
			fmt.Printf("Total: %d Failed: %d\n", total, failed)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func getReq(s string) *api.Request {
	m := isQuery.FindStringSubmatch(s)
	if len(m) > 1 {
		qm := queryRe.FindStringSubmatch(m[1])
		if len(qm) == 0 {
			return nil
		}
		// query := qm[0]
		query, err := strconv.Unquote(qm[0])
		if err != nil {
			fmt.Printf("[ERR] While unquoting query: %v\n %s\n", err, qm[0])
		}
		varStr := varRe.FindAllStringSubmatch(m[1], -1)
		mp := make(map[string]string)
		for _, v := range varStr {
			keys := keyRe.FindStringSubmatch(v[0])
			vals := valRe.FindStringSubmatch(v[0])
			mp[keys[1]] = vals[1]
		}
		return &api.Request{
			Query: query,
			Vars:  mp,
		}
	}
	return nil
}

func getSchema(client *dgo.Dgraph) string {
	txn := client.NewReadOnlyTxn().BestEffort()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := txn.Query(ctx, `schema{}`)
	if err != nil {
		fmt.Printf("[ERR] Got error while querying schema %v", err)
		return "{}"
	}
	return string(resp.Json)
}

func getCounts(client *dgo.Dgraph) {
	var sch Schema
	s := getSchema(client)
	if err := json.Unmarshal([]byte(s), &sch); err != nil {
		log.Fatalf("While unmarshalling schema: %v", err)
		return
	}

	for _, s := range sch.Schema {
		q := fmt.Sprintf("query { f(func: has(%s)) { count(uid) } }", s.Predicate)
		req := &api.Request{Query: q}
		r := runQuery(req, client)

		var cnt map[string]interface{}
		if err := json.Unmarshal([]byte(r), &cnt); err != nil {
			fmt.Errorf("while unmarshalling %v\n", err)
		}
		c := cnt["f"].([]interface{})[0].(map[string]interface{})["count"].(float64)
		fmt.Printf("%-25s ---> %d\n", s.Predicate, int(c))
	}
	return
}

func runQuery(r *api.Request, client *dgo.Dgraph) string {
	txn := client.NewReadOnlyTxn().BestEffort()
	ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Second)
	defer cancel()
	resp, err := txn.QueryWithVars(ctx, r.Query, r.Vars)
	if err != nil {
		fmt.Printf("[ERR] Got error while running %s \n%+v \n%v\n", r.Query, r.Vars, err)
		return "{}"
	}
	return string(resp.Json)
}

func areEqualJSON(s1, s2 string) bool {
	var o1 interface{}
	var o2 interface{}

	var err error
	err = json.Unmarshal([]byte(s1), &o1)
	if err != nil {
		fmt.Errorf("Error mashalling string 1 :: %s", err.Error())
		return false
	}
	err = json.Unmarshal([]byte(s2), &o2)
	if err != nil {
		fmt.Errorf("Error mashalling string 2 :: %s", err.Error())
		return false
	}

	return reflect.DeepEqual(o1, o2)
}
