package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
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
	numGo      uint32
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
	flags.Uint32Var(&opts.numGo,
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

	if opts.countOnly {
		getCounts(dcLeft)
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

type request struct {
	id  uint64
	req *api.Request
}

type response struct {
	req  *request
	resp string
	err  error
}

func processLog(dcLeft, dcRight *dgo.Dgraph) {
	f, err := os.Open(opts.logPath)
	if err != nil {
		klog.Fatalf("While opening log file got error: %v", err)
	}
	defer f.Close()

	var leftMap, rightMap sync.Map
	var reqId, failed, total, leftDone, rightDone uint64
	leftReqCh := make(chan *request, 1000)
	rightReqCh := make(chan *request, 1000)
	leftRespCh := make(chan *response, 1000)
	rightRespCh := make(chan *response, 1000)

	var wg sync.WaitGroup
	worker := func(dc *dgo.Dgraph, inCh chan *request, outCh chan *response) {
		wg.Add(1)
		defer wg.Done()
		for r := range inCh {
			resp, err := runQuery(r.req, dc)
			outCh <- &response{r, resp, err}
		}
	}

	for i := 0; i < 16; i++ {
		go worker(dcLeft, leftReqCh, leftRespCh)
		go worker(dcRight, rightReqCh, rightRespCh)
	}

	go func() {
		wg.Add(1)
		defer wg.Done()
		scan := bufio.NewScanner(f)
		for scan.Scan() {
			r, err := getReq(scan.Text())
			if err != nil {
				// skipping the log line which doesn't have a valid query
				continue
			}
			req := request{reqId, r}
			reqId++
			leftReqCh <- &req
			rightReqCh <- &req
		}
		close(leftReqCh)
		close(rightReqCh)
	}()

	go func() {
		wg.Add(1)
		defer wg.Done()
		for {
			select {
			case res := <-leftRespCh:
				atomic.AddUint64(&leftDone, 1)
				if v, ok := rightMap.Load(res.req.id); ok {
					if !areEqualJSON(res.resp, v.(string)) {
						atomic.AddUint64(&failed, 1)
						klog.Infof("Failed Query: %s \nVars: %v\nLeft: %v\nRight: %v\n",
							res.req.req.Query, res.req.req.Vars, res.resp, v)
					}
					atomic.AddUint64(&total, 1)
					rightMap.Delete(res.req.id)
				} else {
					leftMap.Store(res.req.id, res.resp)
				}
			case res := <-rightRespCh:
				atomic.AddUint64(&rightDone, 1)
				if v, ok := leftMap.Load(res.req.id); ok {
					if !areEqualJSON(res.resp, v.(string)) {
						atomic.AddUint64(&failed, 1)
						klog.Infof("Failed Query: %s \nVars: %v\nLeft: %v\nRight: %v\n",
							res.req.req.Query, res.req.req.Vars, v, res.resp)
					}
					atomic.AddUint64(&total, 1)
					leftMap.Delete(res.req.id)
				} else {
					rightMap.Store(res.req.id, res.resp)
				}
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for {
			select {
			case <-ticker.C:
				klog.Infof("Total: %d Failed: %d leftDone: %d rightDone: %d\n",
					atomic.LoadUint64(&total), atomic.LoadUint64(&failed), atomic.LoadUint64(&leftDone),
					atomic.LoadUint64(&rightDone))
			default:
			}
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
		if err := json.Unmarshal([]byte(r), &cnt); err != nil {
			return errors.Errorf("while unmarshalling %v\n", err)
		}
		c := cnt["f"].([]interface{})[0].(map[string]interface{})["count"].(float64)
		klog.Infof("%-50s ---> %d\n", s.Predicate, int(c))
	}
	return nil
}

func runQuery(r *api.Request, client *dgo.Dgraph) (string, error) {
	txn := client.NewReadOnlyTxn().BestEffort()
	ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Second)
	defer cancel()
	resp, err := txn.QueryWithVars(ctx, r.Query, r.Vars)
	if err != nil {
		return "", errors.Errorf("While running query %s %+v  got error %v\n",
			r.Query, r.Vars, err)
	}
	return string(resp.Json), nil
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
