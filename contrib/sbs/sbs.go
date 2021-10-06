package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

var (
	isQuery = regexp.MustCompile(`Got a query: (.*)`)

	Sbs  Command
	opts Options
)

type SchemaEntry struct {
	Predicate string `json:"predicate"`
	Type      string `json:"type"`
}

type Schema struct {
	Schema []*SchemaEntry
}

type Command struct {
	Cmd  *cobra.Command
	Conf *viper.Viper
}

type Options struct {
	alphaLeft   string
	alphaRight  string
	dryRun      bool
	singleAlpha bool
	logPath     string
	matchCount  bool
	queryFile   string
	readOnly    bool
	numGo       int
}

func init() {
	Sbs.Cmd = &cobra.Command{
		Use:   "sbs",
		Short: "A tool to do side-by-side comparison of dgraph clusters",
		RunE:  run,
	}

	flags := Sbs.Cmd.Flags()
	flags.StringVar(&opts.alphaLeft,
		"alpha-left", "", "GRPC endpoint of left alpha")
	flags.StringVar(&opts.alphaRight,
		"alpha-right", "", "GRPC endpoint of right alpha")
	flags.BoolVar(&opts.dryRun,
		"dry-run", false, "Dry-run the query/mutations")
	flags.StringVar(&opts.logPath,
		"log-file", "", "Path of the alpha log file to replay")
	flags.BoolVar(&opts.matchCount,
		"match-count", false, "Get the count and each predicate and verify")
	flags.StringVar(&opts.queryFile,
		"query-file", "", "The query in this file will be shot concurrently to left alpha")
	flags.BoolVar(&opts.readOnly,
		"read-only", true, "In read only mode, mutations are skipped.")
	flags.BoolVar(&opts.singleAlpha,
		"single-alpha", false, "Only alpha-left has to be specified. Should be used to check only "+
			"to validate if the alpha does not crashes on queries/mutations.")
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

	// single query is meant to be run on the left alpha only.
	if len(opts.queryFile) > 0 {
		singleQuery(dcLeft)
		return nil
	}

	// When querying/mutating over a single cluster, there is not alphaRight.
	if opts.singleAlpha {
		if opts.matchCount {
			getCounts(dcLeft, nil)
			return nil
		}
		processLog(dcLeft, nil)
		return nil
	}

	conn2, err := grpc.Dial(opts.alphaRight, grpc.WithInsecure())
	if err != nil {
		klog.Fatalf("While dialing grpc: %v\n", err)
	}
	defer conn2.Close()
	dcRight := dgo.NewDgraphClient(api.NewDgraphClient(conn2))

	if opts.matchCount {
		getCounts(dcLeft, dcRight)
		return nil
	}

	processLog(dcLeft, dcRight)
	return nil
}

func singleQuery(dc *dgo.Dgraph) {
	klog.Infof("Running single query")
	q, err := ioutil.ReadFile(opts.queryFile)
	if err != nil {
		klog.Fatalf("While reading query file got error: %v", err)
	}
	// It will keep on running this query forever, with numGo in-flight requests.
	var wg sync.WaitGroup
	wg.Add(1)
	for i := 0; i < opts.numGo; i++ {
		go func() {
			for {
				r, err := runQuery(&api.Request{Query: string(q)}, dc)
				if err != nil {
					klog.Error(err)
				}
				klog.V(1).Info("Response: %s\n", r.Json)
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
	hist := newHistogram([]float64{0, 0.01, 0.1, 0.5, 1, 1.5, 2})

	reqCh := make(chan *api.Request, opts.numGo*5)

	var wg sync.WaitGroup
	worker := func(wg *sync.WaitGroup) {
		defer wg.Done()
		for r := range reqCh {
			if opts.readOnly && len(r.Mutations) > 0 {
				continue
			}

			if opts.dryRun {
				klog.Infof("Req: %+v", r)
				continue
			}

			atomic.AddUint64(&total, 1)

			if opts.singleAlpha {
				resp, err := runQuery(r, dcLeft)
				if err != nil {
					atomic.AddUint64(&failed, 1)
					klog.Infof("Failed Request: %+v \nResp: %v Err: %v\n", r, resp, err)
				}
				continue
			}

			respL, errL := runQuery(r, dcLeft)
			respR, errR := runQuery(r, dcRight)
			if errL != nil || errR != nil {
				if errL == nil || errR == nil || errL.Error() != errR.Error() {
					atomic.AddUint64(&failed, 1)
					klog.Infof("Failed Request: %+v \nLeft: %v Err: %v\nRight: %v Err: %v\n",
						r, respL, errL, respR, errR)
				}
				continue
			}

			if !areEqualJSON(string(respL.GetJson()), string(respR.GetJson())) {
				atomic.AddUint64(&failed, 1)
				klog.Infof("Failed Request: %+v \nLeft: %v\nRight: %v\n", r, respL, respR)
			}
			lt := float64(respL.Latency.ProcessingNs) / 1e6
			rt := float64(respR.Latency.ProcessingNs) / 1e6
			// Only consider the processing time > 10 ms for histogram to avoid noise in ratios.
			if lt > 10 || rt > 10 {
				ratio := rt / lt
				hist.add(ratio)
			}
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
			hist.show()
		}
	}()
	wg.Wait()
	klog.Infof("Total: %d Failed: %d\n", atomic.LoadUint64(&total),
		atomic.LoadUint64(&failed))
	hist.show()
}

func getReq(s string) (*api.Request, error) {
	m := isQuery.FindStringSubmatch(s)
	if len(m) > 1 {
		var req api.Request
		if err := proto.UnmarshalText(m[1], &req); err != nil {
			return nil, errors.Wrapf(err, "cannot unmarshal the query log")
		}
		// Allow alpha to lease out the timestamps for the requests otherwise there will be issues
		// as zero does not know about these transactions.
		req.StartTs = 0
		req.CommitNow = true
		return &req, nil
	}
	return nil, errors.Errorf("Not a valid query found in the string")
}

func getSchema(client *dgo.Dgraph) string {
	txn := client.NewReadOnlyTxn().BestEffort()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := txn.Query(ctx, `schema{}`)
	if err != nil {
		klog.Errorf("Got error while querying schema %v", err)
		return "{}"
	}
	return string(resp.Json)
}

func getCounts(left, right *dgo.Dgraph) {
	s := getSchema(left)
	var sch Schema
	if err := json.Unmarshal([]byte(s), &sch); err != nil {
		klog.Fatalf("While unmarshalling schema: %v", err)
	}

	parseCount := func(resp *api.Response) int {
		if resp == nil {
			return 0
		}
		var cnt map[string]interface{}
		if err := json.Unmarshal(resp.Json, &cnt); err != nil {
			klog.Errorf("while unmarshalling %v\n", err)
		}
		c := cnt["f"].([]interface{})[0].(map[string]interface{})["count"].(float64)
		return int(c)
	}

	klog.Infof("%-50s ---> %-15s %-15s\n", "PREDICATE", "LEFT", "RIGHT")

	var failed uint32
	var wg sync.WaitGroup

	type cntReq struct {
		pred string
		req  *api.Request
	}
	reqCh := make(chan *cntReq, 5*opts.numGo)

	for i := 0; i < opts.numGo; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range reqCh {
				rLeft, err := runQuery(r.req, left)
				if err != nil {
					klog.Errorf("While running on left alpha: %v\n", err)
				}
				var rRight *api.Response
				if opts.singleAlpha {
					// If processing a single alpha, lets just copy the response.
					rRight = rLeft
				} else {
					var err error
					rRight, err = runQuery(r.req, right)
					if err != nil {
						klog.Errorf("While running on right alpha: %v\n", err)
					}
				}

				cl, cr := parseCount(rLeft), parseCount(rRight)
				if cl != cr {
					atomic.AddUint32(&failed, 1)
				}
				klog.Infof("%-50s ---> %-15d %-15d\n", r.pred, cl, cr)

			}
		}()
	}

	for _, s := range sch.Schema {
		q := fmt.Sprintf("query { f(func: has(%s)) { count(uid) } }", s.Predicate)
		reqCh <- &cntReq{s.Predicate, &api.Request{Query: q}}
	}
	close(reqCh)
	wg.Wait()
	klog.Infof("Done schema count. Failed predicate count: %d\n", failed)
}

func runQuery(r *api.Request, client *dgo.Dgraph) (*api.Response, error) {
	var txn *dgo.Txn
	if len(r.Mutations) == 0 {
		txn = client.NewReadOnlyTxn().BestEffort()
	} else {
		txn = client.NewTxn()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Second)
	defer cancel()
	resp, err := txn.Do(ctx, r)
	if err != nil {
		return nil, errors.Errorf("While running request %+v got error %v\n", r, err)
	}
	return resp, nil
}
