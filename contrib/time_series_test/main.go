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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"google.golang.org/grpc"
)

const maxEventsInDB = 100
const extraAddedEvents = 10

func dropAll(dg *dgo.Dgraph) error {
	err := dg.Alter(context.Background(), &api.Operation{DropAll: true})
	return err
}

func initSchema(dg *dgo.Dgraph) {
	schema := `
	name: string @index(exact) .
	about: string @index(term, fulltext, trigram) .
	type Event {
		name
		about
	}
	`
	err := dg.Alter(context.Background(), &api.Operation{Schema: schema})
	if err != nil {
		log.Fatal(err)
	}

}

func addEvents(dg *dgo.Dgraph, count int) ([]string, error) {
	randomWords := func(n int) string {
		var words string
		for i := 0; i < n; i++ {
			// keeping size random
			wordLength := rand.Intn(15-5) + 5
			word := []byte{}
			for i := 0; i < wordLength; i++ {
				word = append(word, byte(rand.Intn(26)+97))
			}
			if i == 0 {
				words = string(word)
			} else {
				words = words + " " + string(word)
			}

		}

		return words
	}

	quads := ""
	for i := 0; i < count; i++ {
		quads += fmt.Sprintf(`
		_:%d <name> "%s" .
		 _:%d <about> "%s" .
		_:%d <dgraph.type> "Event" .
		`, i, randomWords(1), i, randomWords(15), i)
	}

	mu := &api.Mutation{
		SetNquads: []byte(quads),
		CommitNow: true,
	}
	txn := dg.NewTxn()
	ctx := context.Background()
	defer txn.Discard(ctx)

	res, err := txn.Mutate(ctx, mu)
	if err != nil {
		return nil, err
	}

	var uids []string
	for _, uid := range res.Uids {
		uids = append(uids, uid)
	}

	return uids, nil
}

func deleteEvents(dg *dgo.Dgraph, uids []string) error {
	nquads := ""
	for _, uid := range uids {
		nquads += fmt.Sprintf(`
		<%s> * *.
		`, uid)
	}

	mu := &api.Mutation{
		DelNquads: []byte(nquads),
		CommitNow: true,
	}
	txn := dg.NewTxn()
	ctx := context.Background()
	defer txn.Discard(ctx)
	_, err := txn.Mutate(ctx, mu)
	if err != nil {
		return err
	}
	return nil
}

func hasQuery(dg *dgo.Dgraph) {
	q := "{ me(func: has(name)) { count(uid) } }"
	txn := dg.NewReadOnlyTxn()
	var timer x.Timer
	timer.Start()
	res, err := txn.Query(context.Background(), q)
	timer.Record("HasQuery time")
	glog.V(2).Infof("Time taken : %s", timer.String())
	if timer.Total().Microseconds() > 2000 {
		glog.Warning("Has query took ", timer.String())
	}
	if err != nil {
		log.Fatal(err)
	}
	glog.V(2).Infof("Result of has query: %v \n", res)
}

func readUID(dg *dgo.Dgraph, uid string) {
	q := fmt.Sprintf("{ me(func: uid(%s)) { name about uid } }", uid)
	txn := dg.NewReadOnlyTxn()
	res, err := txn.Query(context.Background(), q)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%v \n", res)
}

func distinctRandoms(count int, upperLimit int) []int {
	var isPresent map[int]bool = make(map[int]bool)
	var randoms []int
	for i := 0; i < count; {
		time.Sleep(10 * time.Millisecond)
		x := rand.Intn(upperLimit)
		println(x)
		if _, ok := isPresent[x]; ok {
			continue
		}
		isPresent[x] = true
		randoms = append(randoms, x)
		i++
	}

	return randoms
}

func usage() {
	flag.PrintDefaults()
	os.Exit(2)
}

func init() {
	flag.Usage = usage
	flag.Set("logtostderr", "true")
	flag.Set("stderrthreshold", "WARNING")
	flag.Set("v", "2")
	// This is wa
	flag.Parse()
}

func main() {
	conn, err := grpc.Dial("localhost:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	dc := api.NewDgraphClient(conn)
	dg := dgo.NewDgraphClient(dc)
	dropAll(dg)
	initSchema(dg)
	allUids, err := addEvents(dg, maxEventsInDB)
	glog.V(2).Infoln("Added intial events")
	for {
		time.Sleep(10 * time.Millisecond)
		uids, err := addEvents(dg, extraAddedEvents)
		glog.V(2).Infoln("Added extra events")
		allUids = append(allUids, uids...)
		glog.V(2).Infoln("Finding randoms")
		randoms := distinctRandoms(len(allUids)-maxEventsInDB, len(allUids))
		var delUids []string
		for _, num := range randoms {
			delUids = append(delUids, allUids[num])
		}
		if err != nil {
			continue
		}
		err = deleteEvents(dg, delUids)
		if err != nil {
			// retry once
			deleteEvents(dg, uids)
		}
		time.Sleep(10 * time.Millisecond)

		hasQuery(dg)
		break
	}
}
