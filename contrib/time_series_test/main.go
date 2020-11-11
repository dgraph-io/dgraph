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

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"google.golang.org/grpc"
)

// Configurable parameters
const batchSize = 1000
const totalBatches = 1000
const maxEventsInDB = batchSize * 1000
const extraAddedEvents = 1000
const alphAddress = "localhost:9080"

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

func deleteEvents(dg *dgo.Dgraph, randoms map[int]bool, allUids []string) ([]string, error) {
	nquads := ""
	var updatedUids []string

	for i, uid := range allUids {
		if _, ok := randoms[i]; ok {
			nquads += fmt.Sprintf(`
		<%s> * *.
		`, uid)
		} else {
			updatedUids = append(updatedUids, uid)
		}
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
		return nil, err
	}
	return updatedUids, nil
}

type Response struct {
	Me []struct {
		Count int
	}
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
	// var response Response
	// json.Unmarshal([]byte(res.Json), &response)

	// fmt.Printf("%v", response)

	// return response.Me[0].Count
}

func distinctRandoms(count int, upperLimit int) map[int]bool {
	var isPresent map[int]bool = make(map[int]bool)
	var randoms map[int]bool = make(map[int]bool)
	for i := 0; i < count; {
		x := rand.Intn(upperLimit)
		if _, ok := isPresent[x]; ok {
			continue
		}
		isPresent[x] = true
		randoms[x] = true
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
	conn, err := grpc.Dial(alphAddress, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	dc := api.NewDgraphClient(conn)
	dg := dgo.NewDgraphClient(dc)
	dropAll(dg)
	initSchema(dg)
	var allUids []string
	for i := 0; i < totalBatches; i++ {
		uids, err := addEvents(dg, batchSize)
		if err != nil {
			continue
		}
		glog.V(2).Info("Added batch", i+1)
		allUids = append(allUids, uids...)
	}

	glog.V(2).Infoln("Added intial events")
	for {
		uids, err := addEvents(dg, extraAddedEvents)
		glog.V(2).Infoln("Added extra events")
		allUids = append(allUids, uids...)
		glog.V(2).Infoln("Finding randoms")
		indicesToBeDeleted := distinctRandoms(len(allUids)-maxEventsInDB, len(allUids))
		if err != nil {
			continue
		}
		updatedUids, err := deleteEvents(dg, indicesToBeDeleted, allUids)
		if err != nil {
			glog.Errorln(err)
			continue
		}
		allUids = updatedUids
		hasQuery(dg)
	}
}
