package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"

	"github.com/dgraph-io/dgo/v210/protos/api"
)

var (
	isQuery = regexp.MustCompile(`Got a query: query:(.*)`)
	queryRe = regexp.MustCompile(`".*?"`)
	varRe   = regexp.MustCompile(`vars:<.*?>`)
	keyRe   = regexp.MustCompile(`key:(".*?")`)
	valRe   = regexp.MustCompile(`value:(".*?")`)
)

func main() {
	filePath := flag.String("file", "", "Path of the log file")
	flag.Parse()

	f, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("While opening log file got error: %v", err)
	}
	defer f.Close()

	scan := bufio.NewScanner(f)

	for scan.Scan() {
		line := scan.Text()
		r := getReq(line)
		if r != nil {
			fmt.Printf("Query: %s vars: %+v\n", r.Query, r.Vars)
		}
	}
}

func getReq(s string) *api.Request {
	m := isQuery.FindStringSubmatch(s)
	if len(m) > 1 {
		qm := queryRe.FindStringSubmatch(m[1])
		if len(qm) == 0 {
			return nil
		}
		query, err := strconv.Unquote(qm[0])
		if err != nil {
			log.Printf("[ERR] While unquoting query: %v\n", err)
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
