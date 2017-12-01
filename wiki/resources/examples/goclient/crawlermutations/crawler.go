/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/*


*** Check out the blog post about this code : https://open.dgraph.io/post/client0.8.0 ***


This program is an example of how to use the Dgraph go client.  It is one of two example crawlers.
The other, crawlerRDF, is more focused on queries in the Dgraph client.  Look there if you want to
learn about queries and Unmarshal().  This crawler is about mutations using the client interface.

The Dgraph client interface has docs here : https://godoc.org/github.com/dgraph-io/dgraph/client



When we made our tour (https://tour.dgraph.io/) we needed a dataset users could load quickly,
but with enough complexity to demostrate interesting queries.  We decided a subset of
our 21million dataset was just right.  But we can't just take the first n-lines from the
input file - who knows what connections in the graph we'd get or miss if we did that.  It's
a graph; so the best way to make a subset is to crawl it.

This program crawls a Dgraph database loaded with the 21million movie dataset; data available
here : https://github.com/dgraph-io/benchmarks/tree/master/data.  At Dgraph we use this dataset
for the examples in our docs.  There's instructions on starting up Dgraph and loading this
dataset in our docs here : https://docs.dgraph.io/get-started/.

Given a loose bound on the number of edges in the output graph (edgeBound), this crawler crawls by
movies, keeping a queue of unvisited movies.  For each movie it takes off the queue, it
gets all the info for the movie and then queues all movies for all actors in the movie.  The crawl
stops when the edge bound is exceeded (though crawlers complete the their current movie, so it will
blow the bound by a bit).

This crawler crawls one Dgraph instance and stores directly to another instance.  So to run it,
you'll need a source Dgraph instance (or cluster) loaded with the 21million data and a target
instance (or cluster) to load into.


build with

go build crawler.go

run with --help to see the options

Depending on where you've started the Dgraph instances, you might run with something like:

./crawler.go --source "<somehost>:9080,<someotherhost>:9080" --target "127.0.0.1:9080" --edges 500000 --crawlers 10 > crawl.log 2>&1

The ending '> crawl.log 2>&1' redirects logging output to a file.

*** Check out the blog post about this code : https://open.dgraph.io/post/client0.8.0 ***


*/

package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	//"google.golang.org/grpc/metadata"	// for debug mode
	//"github.com/gogo/protobuf/proto"	// for with fmt.Printf("Raw Response: %+v\n", proto.MarshalTextString(resp))

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos/api"
)

var (
	dgraphSource = flag.String("source", "127.0.0.1:9080", "Source Dgraph gRPC server addresses (comma separated)")
	dgraphTarget = flag.String("target", "127.0.0.1:9081", "Target Dgraph gRPC server addresses (comma separated)")
	edgeBound    = flag.Int("edges", 50000, "Stop crawling after we've made this many edges")
	crawlers     = flag.Int("crawlers", 1, "Number of concurrent crawlers")
)

// Dgraph will unpack queries into these structures with client.Unmarshal(),
// we just need to say what goes where.
//
// See how Unmarshal is used in readMovie() and visitMovie().  Godocs and smaller examples are at
// https://godoc.org/github.com/dgraph-io/dgraph/client#Unmarshal
//
// See also our crawlerRDF example for more description about Unmarshal and structs.
type namedNode struct {
	ID   uint64 `json:"uid"`
	Name string `json:"name@en"`
}

type movie struct {
	ReleaseDate time.Time      `json:"initial_release_date"`
	ID          uint64         `json:"uid"`
	Name        string         `json:"EnglishName"`
	NameDE      string         `json:"GermanName"`
	NameIT      string         `json:"ItalianName"`
	Genres      []*namedNode   `json:"genre"`
	Starring    []*performance `json:"starring"`
	Directors   []*namedNode   `json:"~director.film"`
}

type performance struct {
	Actor     *namedNode `json:"performance.actor"`
	Character *namedNode `json:"performance.character"`
}

// A helper struct for Unmarshal in readMovies() and visitMovie().  In visitMovie we unpack the
// query into the whole struct, but Unmarshal() doesn't need to fill the whole
// struct.  The query in readMovies() only fills the ID.  Another option in readMovies() would
// be reading into a namedNode or just grabbing the UID like in visitActor().
type movieQuery struct {
	Root *movie `json:"movie"`
}

// Tick off the things we have seen in our crawl so far, so we only create each movie, director,
// actor, etc. once in the target.
type savedNodes struct {
	nodes        map[uint64]client.Node // source UID -> target Node
	sync.RWMutex                        // many concurrent crawlers will need safe access to the map
}

var (

	// Start the crawl here.
	movieSeeds = []string{"Blade Runner"}

	// Queue of movies to visit.
	toVisit = make(chan uint64, *crawlers)

	// Movies we've seen so far in the crawl.  If a movie is ticked as true here, a crawler has
	// seen that it exists and added it to toVisit.  A movie is only ever added once to toVisit.
	movies = make(map[uint64]bool)
	movMux sync.Mutex

	// Actors can also be directors, so we need to keep track of them together.
	people = savedNodes{nodes: make(map[uint64]client.Node)}

	// The same character in two different movies is the same character node in the graph - the
	// node for character Harry Potter is the same for Daniel Radcliffe in all Harry Potter movies
	// and even for Toby Papworth who played the baby Harry.  So as well as making a single node
	// for Daniel Radcliffe that's used for all his roles, we also need to ensure we link the
	// characters correctly.
	characters = savedNodes{nodes: make(map[uint64]client.Node)}

	// Genres seen.
	genres = savedNodes{nodes: make(map[uint64]client.Node)}

	edgeCount int32 // = 0

	// To signal crawlers to stop when all is done.
	done = make(chan interface{})

	wg sync.WaitGroup // Crawlers signal main go routine as they exit.

	// Queries with variable and associated variable map.  For use with SetQueryWithVariables().
	// See also https://docs.dgraph.io/query-language/#graphql-variables.
	// Queries with variables allow reusing a query without having to modify
	// the raw string.  The query string can remain unchanged and the variable
	// map changed.
	movieByNameTemplate = `{
	movie(func: eq(name@en, $a)) @filter(has(genre)) {
		uid
	}
}`
	movieByNameMap = make(map[string]string)

	movieByIDTemplate = `{
	movie(func: uid($a)) {
		uid
		EnglishName: name@en
		GermanName: name@de
		ItalianName: name@it
      	starring {
        	performance.actor {
				uid
        		name@en
        	}
			performance.character {
				uid
				name@en
			}
		}
		genre {
			uid
			name@en
		}
		~director.film {
			uid
			name@en
		}
		initial_release_date
	}
}`
	movieByIDMap = make(map[string]string)

	// A query without variables.  For this one, we'll manipulate the raw string by printing
	// straight into %v with fmt.Sprintf().
	actorByIDTemplate = `{
	actor(func: uid(%v)) {
		actor.film {
			performance.film {
				uid
			}
		}
	}
}`
)

func getContext() context.Context {
	return context.Background()
	//return metadata.NewContext(context.Background(), metadata.Pairs("debug", "true"))
}

// Setup a request and run a query with variables.
func runQueryWithVariables(dgraphClient *client.Dgraph, query string, varMap map[string]string) (*api.Response, error) {
	req := client.Req{}
	req.SetQueryWithVariables(query, varMap)
	return dgraphClient.Run(getContext(), &req)
}

// ---------------------- crawl movies ----------------------

// readMovies enqueues the seed movies into the queue (toVisit) ready to be crawlled.
func readMovies(movies []string, dgraphClient *client.Dgraph) {
	for _, mov := range movies {
		log.Print("Finding movie : ", mov)

		movieByNameMap["$a"] = mov
		resp, err := runQueryWithVariables(dgraphClient, movieByNameTemplate, movieByNameMap)
		if err != nil {
			log.Printf("Finding movie %s.  --- Error in getting response from server, %s.", mov, err)
			// But still continue if we fail with some movies.
		} else {
			var m movieQuery
			err = client.Unmarshal(resp.N, &m)
			if err == nil {
				log.Print("Found Movie ", m.Root.ID)
				enqueueMovie(m.Root.ID)
			} else {
				log.Printf("Couldn't unmarshal response for %s.", mov)
				log.Printf(err.Error())
			}
		}
	}
}

// ensureNamedNodes makes sure that the given named node (from the source) is created in the
// target Dgraph instance and ticked off in our source UID -> target node map.  If anything goes
// wrong, it just returns an error - and hasn't either saved in the target instance or in the map.
//
// Some bookkeeping is required in this crawl to ensure that movies are only visited once and
// that each actor is queried for only once.  Example movielensbatch shows how the client can
// reduce this burden by associating a label to nodes.
func ensureNamedNode(n *namedNode, saved *savedNodes, target *client.Dgraph) error {

	saved.RLock()
	if _, ok := saved.nodes[n.ID]; !ok { // optimisitic first read

		// read failed, so we need a write lock to update
		saved.RUnlock()
		saved.Lock()
		defer saved.Unlock()

		// but another go routine might have written in the meantime
		if _, ok := saved.nodes[n.ID]; !ok {

			node, err := target.NodeBlank("")
			if err != nil {
				return err
			}

			req := client.Req{}
			e := node.Edge("name")
			e.SetValueString(n.Name)
			req.Set(e)

			_, err = target.Run(context.Background(), &req)
			if err != nil {
				return err
			}

			saved.nodes[n.ID] = node
		}
	} else {
		// value was already there
		saved.RUnlock()
	}
	return nil
}

// Protect concurrent reads.
func getFromSavedNode(saved *savedNodes, ID uint64) (n client.Node, ok bool) {
	saved.RLock()
	n, ok = saved.nodes[ID]
	saved.RUnlock()
	return
}

// visitMovie is called when a movies's UID is taken off the queue.  It queries all the data
// for the movie and writes to the target.  It also adds any movies of actors in this movie
// to the queue of movies to crawl.
func visitMovie(movieID uint64, source *client.Dgraph, target *client.Dgraph) {

	if atomic.LoadInt32(&edgeCount) < int32(*edgeBound) {

		log.Print("Processing movie : ", movieID)

		movieByIDMap["$a"] = fmt.Sprint(movieID)

		resp, err := runQueryWithVariables(source, movieByIDTemplate, movieByIDMap)
		if err != nil {
			log.Printf("Error processing movie : %v.\n%s", movieID, err)
			return // fall over for this movie, but keep trying others
		}
		// fmt.Printf("Raw Response: %+v\n", proto.MarshalTextString(resp))

		var mq movieQuery
		err = client.Unmarshal(resp.N, &mq)
		if err != nil {
			log.Print("Couldn't unmarshal response for movie ", movieID)
			log.Printf(err.Error())
			return
		}
		m := mq.Root

		log.Println("Found data for movie : ", m.ID, " - ", m.Name)

		var edgesToAdd int32

		// req will store all the edges for this movie (director names, actor names, etc are
		// committed separately by ensureNamedNode()).  We'll committ all at once or fail
		// for the whole movie.
		req := client.Req{}

		// Every edge we are going to committ will be stored in here.  Once we've called
		// req.Set(e), it's safe to reuse e for a different edge.
		var e client.Edge

		// Getting a blank node with an empty string just gets a new node and doesn't record
		// a blank node name to node map.  Here, we don't need the blank nodes outside of this
		// function, so no need to map them for later use.
		mnode, err := target.NodeBlank("")
		if err != nil {
			log.Println("Failing for movie : ", m.ID, " - ", m.Name)
			log.Println(err)
			return
		}

		if m.Name != "" {
			e = mnode.Edge("name")
			err = e.SetValueStringWithLang(m.Name, "en")
			if err != nil {
				log.Println("Failing for movie : ", m.ID, " - ", m.Name)
				log.Println(err)
				return
			}
			err = req.Set(e)
			if err != nil {
				log.Println("Failing for movie : ", m.ID, " - ", m.Name)
				log.Println(err)
				return
			}
			edgesToAdd++
		}
		if m.NameDE != "" {
			e = mnode.Edge("name")
			if e.SetValueStringWithLang(m.NameDE, "de") == nil {
				req.Set(e)
				edgesToAdd++
			} // ignore this edge if error
		}
		if m.NameIT != "" {
			e = mnode.Edge("name@it")
			if e.SetValueStringWithLang(m.NameIT, "it") == nil {
				req.Set(e)
				edgesToAdd++
			}
		}
		if !m.ReleaseDate.IsZero() {
			e = mnode.Edge("initial_release_date")
			if e.SetValueDatetime(m.ReleaseDate) == nil {
				req.Set(e)
				edgesToAdd++
			}
		}

		// A movie can have a number of directors, so we'll add one edge for each one.
		// If we haven't seen the director before, add their name too and update the
		// directors map.
		edgesToAdd += int32(len(m.Directors))
		for _, d := range m.Directors {
			err = ensureNamedNode(d, &people, target)
			if err != nil {
				log.Println("Failing for movie : ", m.ID, " - ", m.Name)
				log.Println(err)
				return
			}

			dnode, _ := getFromSavedNode(&people, d.ID)
			e = dnode.ConnectTo("director.film", mnode)
			req.Set(e)
		}

		// A movie can have a number of genres.  We'll add an edge for each one, but,
		// first, ensure that each genre is added.
		edgesToAdd += int32(len(m.Genres))
		for _, g := range m.Genres {
			err = ensureNamedNode(g, &genres, target)
			if err != nil {
				log.Println("Failing for movie : ", m.ID, " - ", m.Name)
				log.Println(err)
				return
			}

			gnode, _ := getFromSavedNode(&genres, g.ID)
			e = mnode.ConnectTo("genre", gnode)
			req.Set(e)
		}

		// The movies for all actors in this movie.  After we're done, we'll enqueue all of these.
		newMoviesToVisit := make([]uint64, 1000)

		// Each movie has a number of performances, which record an actor playing the role of a
		// particular character in the movie.  We need to add edges
		// m.ID --- starring ---> p
		// p --- performance.actor ---> actor
		// p --- performance.character ---> character
		// for every character in the movie.  And add the actors and characters, if they haven't
		// been added yet.
		for _, p := range m.Starring {
			if p.Actor != nil && p.Character != nil {
				edgesToAdd++ // for m.ID --- starring ---> p
				pnode, err := target.NodeBlank("")
				if err != nil {
					log.Println("Failing for movie : ", m.ID, " - ", m.Name)
					log.Println(err)
					return
				}
				e = mnode.ConnectTo("starring", pnode)
				req.Set(e)

				edgesToAdd++ // for p --- performance.character ---> p.character
				err = ensureNamedNode(p.Character, &characters, target)
				if err != nil {
					log.Println("Failing for movie : ", m.ID, " - ", m.Name)
					log.Println(err)
					return
				}
				cnode, _ := getFromSavedNode(&characters, p.Character.ID)
				e = pnode.ConnectTo("performance.character", cnode)
				req.Set(e)

				// Only get the movies of this actor if they haven't been seen before.
				if _, ok := getFromSavedNode(&people, p.Actor.ID); !ok {
					newMoviesToVisit = append(newMoviesToVisit, visitActor(p.Actor, source)...)
				}

				edgesToAdd++ // for p --- performance.actor ---> p.Actora.ID
				err = ensureNamedNode(p.Actor, &people, target)
				if err != nil {
					log.Println("Failing for movie : ", m.ID, " - ", m.Name)
					log.Println(err)
					return
				}
				anode, _ := getFromSavedNode(&people, p.Actor.ID)
				e = pnode.ConnectTo("performance.actor", anode)
				req.Set(e)
			}
		}

		// If we got this far, all the people, genres and characters have been saved to the
		// store and req contains Set mutations for all the edges in this movie.
		_, err = target.Run(getContext(), &req)
		if err != nil {
			// Not worrying about potential errors here, just ignore this movie if Run fails
		} else {
			// If all those edges were saved, safely increase the edge count
			atomic.AddInt32(&edgeCount, edgesToAdd)
			log.Print("Committed movie : ", m.ID, " - ", m.Name)
		}

		for _, id := range newMoviesToVisit {
			enqueueMovie(id)
		}

	} else {
		log.Print("Too many edges.  Ignoring movie : ", movieID)
	}
}

// visitActor is called each time an actor is seen.  Try to only run it once for each actor.  But
// if multiple go routines find the same actor at the same time, we might get through the check,
// but enqueueMovie() is safe, so it's just a wasted query.
func visitActor(a *namedNode, dgraphClient *client.Dgraph) (result []uint64) {

	log.Println("visiting actor : ", a.ID, " - ", a.Name)

	// Find all the movies this actor has worked on
	req := client.Req{}
	req.SetQuery(fmt.Sprintf(actorByIDTemplate, a.ID))
	resp, err := dgraphClient.Run(getContext(), &req)
	if err != nil {
		log.Printf("Error processing actor : %v (%v).  --- Error in getting response from server, %s", a.Name, a.ID, err)
		return result
	}

	// - resp.N should be length 1 because there was only 1 query block
	// - resp.N[0].Children should be length 1 because the query asked by UID, so only one node
	//   can be in the answer
	if len(resp.N) == 1 && len(resp.N[0].Children) == 1 {

		// We are at an actor
		// For each performance in each movie they have acted in - actor.film
		for _, actorsMovie := range resp.N[0].Children[0].Children {

			// At a performance.
			// Really, should only be one edge to a film from a performance - performance.film
			for _, actorPerformance := range actorsMovie.Children {

				// At a film.
				// We've walked the graph response from actor to the films they have acted in.

				var uid uint64

				for _, prop := range actorPerformance.Properties {
					switch prop.Prop {
					case "uid":
						uid = prop.GetValue().GetUidVal()
					}
				}
				result = append(result, uid)
			}
		}
	}
	return result
}

// Add the movie to the queue of movies to visit if we haven't blown the edge bound.  If we have
// gone over the edge bound, signal all crawlers to exit.
func enqueueMovie(movID uint64) {
	movMux.Lock()
	defer movMux.Unlock()

	// We have to lock here to protect against visiting the same movie multiple times.
	// So we can safely close done if it hasn't been closed before.
	//
	// We can't close toVisit as the exit signal because there is no guarantee that the spawned
	// go routines that write to it have done so, even if they were started when safe to do so.

	if atomic.LoadInt32(&edgeCount) > int32(*edgeBound) {
		select {
		case <-done:
			// noop - already closed
		default:
			log.Print("Sending exit signal.")
			close(done)
		}
		return
	}

	if _, ok := movies[movID]; !ok {
		movies[movID] = true
		go func() { toVisit <- movID }()
	}
}

func startCrawler(source *client.Dgraph, target *client.Dgraph) {

	defer wg.Done() // no matter what happens signal my exit

	for {
		select {
		case movID := <-toVisit:
			visitMovie(movID, source, target)
		case <-done:
			return
		}
	}
}

func crawl(numCrawlers int, source *client.Dgraph, target *client.Dgraph) {

	wg.Add(numCrawlers)

	for i := 0; i < numCrawlers; i++ {
		go startCrawler(source, target)
	}

	wg.Wait()

	// After wg.Wait() is done, all the reading and writing is done, so it's safe to exit.
}

// Not quite higher-order compose.
// Compose together the exit functions and log any errors.
func compose(f func(), g func() error) func() {
	return func() {
		err := g()
		if err != nil {
			log.Print("Shutdown not clean : ", err)
		}
		f()
	}
}

// When connecting to a Dgraph client you can specify a slice of grpc connections.  That way
// you can open multiple connections - even connections to multiple machines if you are running
// in a cluster - and the client will spread your requests accross the connections.
func connect(dgraphs []string) (*client.Dgraph, func(), error) {

	// A function to do all the clean up from this connection.
	// If we fail partway through, call this do undo everything and return the error.
	f := func() {}

	var conns []*grpc.ClientConn
	for _, host := range dgraphs {
		conn, err := grpc.Dial(host, grpc.WithInsecure())
		if err != nil {
			f()
			return nil, nil, err
		}
		conns = append(conns, conn)

		f = compose(f, conn.Close)
	}

	// The Dgraph client needs a directory in which to write its blank node and XID maps.
	clientDir, err := ioutil.TempDir("", "client_")
	if err != nil {
		f()
		return nil, nil, err
	}
	f = compose(f, func() error { return os.RemoveAll(clientDir) })

	// A DgraphClient made from a slice of succeful connections.  All calls to the client
	// and thus the backend(s) go through this.  We won't be doing any batching with these
	// clients, just queries and running mutations, so we can ignore the BatchMutationOptions.
	dg := client.NewDgraphClient(conns, client.DefaultOptions, clientDir)
	f = compose(f, dg.Close)

	return dg, f, nil
}

// ---------------------- main----------------------

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()
	if !flag.Parsed() {
		log.Fatal("Unable to parse flags")
	}

	srcHostList := strings.Split(*dgraphSource, ",")
	if len(srcHostList) == 0 {
		log.Fatal("No source Dgraph given")
	}
	dgraphSource, srcCleanup, err := connect(srcHostList)
	if err != nil {
		log.Fatal(err)
	}
	defer srcCleanup()

	destHostList := strings.Split(*dgraphTarget, ",")
	if len(destHostList) == 0 {
		log.Fatal("No target Dgraph given")
	}
	dgraphTarget, trgtCleanup, err := connect(destHostList)
	if err != nil {
		log.Fatal(err)
	}
	defer trgtCleanup()

	// Add a schema for the target Dgraph instance.  The original schema has edges for
	// performance.film (the reverse of starrting)
	// performance.actor (the reverse of actor.film)
	// For this example, we'll just rely on Dgraph to create the reverse edges.
	req := client.Req{}
	req.SetQuery(`mutation {
	schema {
		director.film        : uid @reverse @count .
		actor.film           : uid @reverse @count .
		genre                : uid @reverse @count .
		starring			 : uid @reverse @count .
		initial_release_date : dateTime @index .
		name                 : string @index(term, exact, fulltext, trigram) .
	}
}`)
	_, err = dgraphTarget.Run(getContext(), &req)
	if err != nil {
		log.Fatal("Couldn't set target schema.\n", err)
	}

	// Fill up the toVisit queue with movie IDs
	readMovies(movieSeeds, dgraphSource)

	crawl(*crawlers, dgraphSource, dgraphTarget)

	log.Printf("Wrote %v people (actors and directors).", len(people.nodes))
	log.Printf("Wrote %v movies.", len(movies))
	log.Printf("Wrote %v characters.", len(characters.nodes))
	log.Printf("Wrote %v genres.", len(genres.nodes))

	// More edges than this will end up in the store if the schema has @reverse edges.
	log.Printf("Wrote %v edges in total.", edgeCount)

}
