package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

var (
	isQuery = regexp.MustCompile(`Got a query: query:(.*)`)
	queryRe = regexp.MustCompile(`".*?"`)
	varRe   = regexp.MustCompile(`vars:<.*?>`)
	keyRe   = regexp.MustCompile(`key:"(.*?)"`)
	valRe   = regexp.MustCompile(`value:"(.*?)"`)

	Sbs  Command
	opts Options
)

type SchemaEntry struct {
	Predicate string `json="predicate"`
	Type      string `json="type"`
}

type Schema struct {
	Schema []*SchemaEntry
}

type Command struct {
	Cmd  *cobra.Command
	Conf *viper.Viper
}

type Options struct {
	logPath    string
	alphaLeft  string
	alphaRight string
	countOnly  bool
	queryFile  string
	numGo      int
}

func init() {
	Sbs.Cmd = &cobra.Command{
		Use:   "sbs",
		Short: "A tool to do side-by-side comparision of dgraph clusters",
		RunE:  run,
	}

	flags := Sbs.Cmd.Flags()
	flags.StringVar(&opts.logPath,
		"log-file", "", "Path of the alpha log file to replay")
	flags.StringVar(&opts.alphaLeft,
		"alpha-left", "", "GRPC endpoint of left alpha")
	flags.StringVar(&opts.alphaRight,
		"alpha-right", "", "GRPC endpoint of right alpha")
	flags.BoolVar(&opts.countOnly,
		"counts-only", false, "Only get the count of all predicates in the left alpha")
	flags.StringVar(&opts.queryFile,
		"query-file", "", "The query in this file will be shot concurrently to left alpha")
	flags.IntVar(&opts.numGo,
		"workers", 16, "Number of query request workers")
	Sbs.Conf = viper.New()
	Sbs.Conf.BindPFlags(flags)

	fs := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(fs)
	Sbs.Cmd.Flags().AddGoFlagSet(fs)
}

func main() {
	flag.CommandLine.Set("logtostderr", "true")
	check(flag.CommandLine.Parse([]string{}))
	check(Sbs.Cmd.Execute())
}

func run(cmd *cobra.Command, args []string) error {
	conn, err := grpc.Dial(opts.alphaLeft, grpc.WithInsecure())
	if err != nil {
		klog.Fatalf("While dialing grpc: %v\n", err)
	}
	defer conn.Close()
	dcLeft := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	// counts only and single query are meant to be run on the left alpha only.
	if opts.countOnly {
		getCounts(dcLeft)
		return nil
	}
	if len(opts.queryFile) > 0 {
		singleQuery(dcLeft)
		return nil
	}

	conn2, err := grpc.Dial(opts.alphaRight, grpc.WithInsecure())
	if err != nil {
		klog.Fatalf("While dialing grpc: %v\n", err)
	}
	defer conn2.Close()
	dcRight := dgo.NewDgraphClient(api.NewDgraphClient(conn2))

	processLog(dcLeft, dcRight)
	return nil
}

func singleQuery(dc *dgo.Dgraph) {
	klog.Infof("Running single query")
	q, err := ioutil.ReadFile(opts.queryFile)
	if err != nil {
		klog.Fatalf("While reading query file got error: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < opts.numGo; i++ {
		wg.Add(1)
		go func() {
			for {
				r, err := runQuery(&api.Request{Query: string(q)}, dc)
				if err != nil {
					klog.Error(err)
				}
				fmt.Println(string(r.Json))
			}
		}()
	}
	wg.Wait()
}

func processLog(dcLeft, dcRight *dgo.Dgraph) {
	f, err := os.Open(opts.logPath)
	if err != nil {
		klog.Fatalf("While opening log file got error: %v", err)
	}
	defer f.Close()

	var failed, total uint64
	reqCh := make(chan *api.Request, opts.numGo*5)

	var wg sync.WaitGroup
	worker := func(wg *sync.WaitGroup) {
		defer wg.Done()
		for r := range reqCh {
			respL, err := runQuery(r, dcLeft)
			if err != nil {
				klog.Errorf("While running on left: %v\n", err)
			}
			respR, err := runQuery(r, dcRight)
			if err != nil {
				klog.Errorf("While running on right: %v\n", err)
			}
			if !areEqualJSON(string(respL.Json), string(respR.Json)) {
				atomic.AddUint64(&failed, 1)
				klog.Infof("Failed Query: %s \nVars: %v\nLeft: %v\nRight: %v\n",
					r.Query, r.Vars, respL, respR)
			}
			atomic.AddUint64(&total, 1)
		}
	}

	for i := 0; i < opts.numGo; i++ {
		wg.Add(1)
		go worker(&wg)
	}

	go func() {
		scan := bufio.NewScanner(f)
		for scan.Scan() {
			r, err := getReq(scan.Text())
			if err != nil {
				// skipping the log line which doesn't have a valid query
				continue
			}
			reqCh <- r
		}
		close(reqCh)
	}()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for range ticker.C {
			klog.Infof("Total: %d Failed: %d\n", atomic.LoadUint64(&total),
				atomic.LoadUint64(&failed))
		}
	}()
	wg.Wait()
}

func getReq(s string) (*api.Request, error) {
	m := isQuery.FindStringSubmatch(s)
	if len(m) > 1 {
		qm := queryRe.FindStringSubmatch(m[1])
		if len(qm) == 0 {
			return nil, errors.Errorf("Not a valid query found in the string")
		}
		query, err := strconv.Unquote(qm[0])
		if err != nil {
			return nil, errors.Wrap(err, "while unquoting")
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
		}, nil
	}
	return nil, errors.Errorf("Not a valid query found in the string")
}

func getSchema(client *dgo.Dgraph) string {
	txn := client.NewReadOnlyTxn().BestEffort()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := txn.Query(ctx, `schema{}`)
	if err != nil {
		klog.Errorf("[ERR] Got error while querying schema %v", err)
		return "{}"
	}
	return string(resp.Json)
}

func getCounts(client *dgo.Dgraph) error {
	var sch Schema
	s := getSchema(client)
	if err := json.Unmarshal([]byte(s), &sch); err != nil {
		return errors.Errorf("While unmarshalling schema: %v", err)
	}

	for _, s := range sch.Schema {
		q := fmt.Sprintf("query { f(func: has(%s)) { count(uid) } }", s.Predicate)
		req := &api.Request{Query: q}
		r, err := runQuery(req, client)
		if err != nil {
			return errors.Wrap(err, "While running query")
		}

		var cnt map[string]interface{}
		if err := json.Unmarshal(r.Json, &cnt); err != nil {
			return errors.Errorf("while unmarshalling %v\n", err)
		}
		c := cnt["f"].([]interface{})[0].(map[string]interface{})["count"].(float64)
		klog.Infof("%-50s ---> %d\n", s.Predicate, int(c))
	}
	return nil
}

func runQuery(r *api.Request, client *dgo.Dgraph) (*api.Response, error) {
	txn := client.NewReadOnlyTxn().BestEffort()
	ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Second)
	defer cancel()
	resp, err := txn.QueryWithVars(ctx, r.Query, r.Vars)
	if err != nil {
		return nil, errors.Errorf("While running query %s %+v  got error %v\n",
			r.Query, r.Vars, err)
	}
	return resp, nil
}

func areEqualJSON(s1, s2 string) bool {
	var o1 interface{}
	var o2 interface{}

	var err error
	err = json.Unmarshal([]byte(s1), &o1)
	if err != nil {
		return false
	}
	err = json.Unmarshal([]byte(s2), &o2)
	if err != nil {
		return false
	}
	return reflect.DeepEqual(o1, o2)
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
