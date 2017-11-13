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

/*


*** Check out the blog post about this code : https://open.dgraph.io/post/client0.8.0 ***


This program is an example of how to use the Dgraph go client.  It is one of two example crawlers.
The other, crawlerMutations, is more focused on running concurrent mutations through the Dgraph
client.  This crawler is about querying and unmarshalling results.

The Dgraph client interface has docs here : https://godoc.org/github.com/dgraph-io/dgraph/client



When we made our tour (https://tour.dgraph.io/) we needed a dataset users could load quickly,
but with enough complexity to demostrate interesting queries.  We decided a subset of
our 21million dataset was just right.  But we can't just take the first n-lines from the
input file - who knows what connections in the graph we'd get or miss if we did that.  It's a graph,
so the best was to make a subset is to crawl it.

This program crawls a dgraph database loaded with the 21million movie dataset; data available
here : https://github.com/dgraph-io/benchmarks/tree/master/data.  At Dgraph we use this dataset
for the examples in our docs.  There's instructions on starting up Dgraph and loading this
dataset in our docs here : https://docs.dgraph.io/get-started/.

Given a loose bound on the number of edges in the output graph (edgeBound), this crawler crawls by
directors, keeping a queue of unvisited directors.  For each director it takes off the queue, it
gets all their movies and then queues the directors that actors in those movies have worked for.
The crawler stops after the director whose movies make the crawl exceed the edge count. This way
the output is complete for movies and directors, but won't have every movie for all the actors the
crawler has found.

When the crawl finishes a gzipped RDF file is written.


Run with '--help' to see options.  You may want to run with '> crawl.log 2>&1' to redirect logging
output to a file.  For example:

./crawler --edges 500000 > crawl.log 2>&1

This crawler outputs gzipped rdf.  The output file can be loaded into a Dgraph instance with

dgraph-live-loader --s <schemafile> --r crawl.rdf.gz

The schema could be the same as 21million (https://github.com/dgraph-io/benchmarks/blob/master/data/21million.schema),
or

director.film        : uid @reverse @count .
actor.film           : uid @count .
genre                : uid @reverse @count .
initial_release_date : datetime @index .
name                 : string @index(term, exact, fulltext, trigram) .


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
	"time"

	"compress/gzip"

	"google.golang.org/grpc"
	//"google.golang.org/grpc/metadata"	// for debug mode
	//"github.com/gogo/protobuf/proto"	// for with fmt.Printf("Raw Response: %+v\n", proto.MarshalTextString(resp))

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
)

var (
	dgraph    = flag.String("d", "127.0.0.1:9080", "Dgraph gRPC server address")
	edgeBound = flag.Int("edges", 50000, "Stop crawling directors after we've made this many edges")
	outfile   = flag.String("o", "crawl.rdf.gz", "Output file")
)

// Types namedNode, director, actor, genre, character, movie and performance
// represent the structures in the movie graph.  A movie has a name, release
// date, unique id, and a list of genres, directors and performances.  A
// performance represents a character being played by an actor in the movie.
// For actors, directors, genres and characters all we are going to crawl is a name.
// The unique ID (the UID in dgraph) is needed throughout, because we need to
// tick off what we have crawled, so we don't visit things twice.
//
// Dgraph will unpack queries into these structures with client.Unmarshal(), we just need to say
// what goes where.
type namedNode struct { // see also query directorByNameTemplate and readDirectors where the query is unmarshalled into this struct
	ID   uint64 `json:"uid"`   // Use `json:"edgename"` to tell client.Unmarshal() where to put which edge in your struct.
	Name string `json:"name@en"` // Struct fields must be exported (have an intial capital letter) to be accesible to client.Unmarshal().
}

type director namedNode
type actor namedNode
type genre namedNode
type character namedNode

type movie struct { // see also query directorsMoviesTemplate and visitDirector where the query is unmarshalled into this struct
	ReleaseDate time.Time      `json:"initial_release_date"` // Often just use the edge name and a reasonable type.
	ID          uint64         `json:"uid"`                // uid is extracted to uint64 just like any other edge.
	Name        string         `json:"EnglishName"`          // If there is an alias on the edge, use the alias.
	NameDE      string         `json:"GermanName"`
	NameIT      string         `json:"ItalianName"`
	Genre       []genre        `json:"genre"`          // The struct types can be nested.  As long as the tags match up, all is well.
	Starring    []*performance `json:"starring"`       // Pointers to structures are fine too - that might save copying structures later.
	Director    []*director    `json:"~director.film"` // reverse edges work just like forward edges.
}

type performance struct {
	Actor     *actor     `json:"performance.actor"`
	Character *character `json:"performance.character"`
}

// Types directQuery and movieQuery help unpack query results with client.Unmarshal().
// The Unmarshal function needs the tags to know how to unpack query results.
// With type director, Unmarshal could unpack a uid and name@en, with directQuery,
// Unmarshal can unpack the whole director, or for movieQuery a slice of movie types
// from a query that returns many results.
type directQuery struct {
	Root director `json:"director"`
}

type movieQuery struct {
	Root []*movie `json:"movie"`
}

var (

	// Start the crawl here.
	directorSeeds = []string{"Peter Jackson", "Jane Campion", "Ang Lee", "Marc Caro",
		"Jean-Pierre Jeunet", "Tom Hanks", "Steven Spielberg",
		"Cherie Nowlan", "Hermine Huntgeburth", "Tim Burton", "Tyler Perry"}

	// Record what we've seen and for output at the end.
	directors  = make(map[uint64]*director)
	actors     = make(map[uint64]*actor)
	characters = make(map[uint64]*character)
	movies     = make(map[uint64]*movie)
	genres     = make(map[uint64]*genre)

	edgeCount = 0

	toVisit chan uint64 // queue of director UIDs to visit

	// Queries with variables and associated variable map.  For use with SetQueryWithVariables().
	// See also https://docs.dgraph.io/query-language/#graphql-variables.
	// Queries with variables allow reusing a query without having to modify
	// the raw string.  The query string can remain unchanged and the variable
	// map changed.  Used in readDirectors() and visitDirector().
	directorByNameTemplate = `{
	director(func: eq(name@en, $a)) @filter(has(director.film)) {
		uid
		name@en
	}
}`
	directorByNameMap = make(map[string]string)

	directorsMoviesTemplate = `{
	movies(func: uid($a)) {
		movie: director.film {
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
	}
}`
	directorMoviesMap = make(map[string]string)

	// A query without variables.  For this one, we'll manipulate the raw
	// string by print straight into %v with fmt.Sprintf().
	// Used in visitActor().
	actorByIDTemplate = `{
	actor(func: uid(%v)) {
		actor.film {
			performance.film {
				~director.film {
					uid
					name@en
				}
			 }
		}
	}
}`
)

// ---------------------- Dgraph helper fns ----------------------

func getContext() context.Context {
	return context.Background()
	//return metadata.NewContext(context.Background(), metadata.Pairs("debug", "true"))
}

// printNode is simply an example of walking through the protocol buffer response.
// It just formats the Node as text.  A node has
// - Atrribute/GetAttribute() the edge that lead to this node
// - Properties/GetProperties() the scalar valued edges out of this node (each property has a name (Prop/GetProp()) and value (Value/GetValue()))
// - Children/GetChildren() the uid edges from this Node to other nodes (the edge name is recorded in the Attribute of the target)
//
// Function proto.MarshalTextString() from github.com/gogo/protobuf/proto already does this job, e.g.,
// fmt.Printf("Raw Response: %+v\n", proto.MarshalTextString(resp))
func printNode(depth int, node *protos.Node) {

	fmt.Println(strings.Repeat(" ", depth), "Atrribute : ", node.Attribute)

	// the values at this level
	for _, prop := range node.GetProperties() {
		fmt.Println(strings.Repeat(" ", depth), "Prop : ", prop.Prop, " Value : ", prop.Value, " Type : %T", prop.Value)
	}

	for _, child := range node.Children {
		fmt.Println(strings.Repeat(" ", depth), "+")
		printNode(depth+1, child)
	}

}

// Setup a request and run a query with variables.
func runQueryWithVariables(dgraphClient *client.Dgraph, query string, varMap map[string]string) (*protos.Response, error) {
	req := client.Req{}
	req.SetQueryWithVariables(query, varMap)
	return dgraphClient.Run(getContext(), &req)
}

// ---------------------- crawl movies ----------------------

// readDirectors enqueues the seed directors into the queue (toVisit) to be crawlled.
func readDirectors(directors []string, dgraphClient *client.Dgraph) {
	for _, dir := range directors {
		log.Print("Finding director : ", dir)

		directorByNameMap["$a"] = dir
		resp, err := runQueryWithVariables(dgraphClient, directorByNameTemplate, directorByNameMap)
		if err != nil {
			log.Printf("Finding director %s.  --- Error in getting response from server, %s.", dir, err)
		} else {

			// client.Unmarshal() unpacks a query result directly into a struct.
			// It's analogous to json.Unmarshal() for json files.
			//
			// resp.N is a slice of Nodes - one Node for each named query block
			// in the query string sent to Dgraph.  Each of those nodes
			// has atribute "_root_" and has children for each graph node returned
			// by the query.  Those child nodes are labelled with the query name.
			//
			// Unmarshal() takes a slice of nodes and searches for children that have
			// an attribute matching the tag of the given struct.
			//
			// In this case that's searching the children of resp.N for atributes matching
			// director - thus matching query director and unmarshalling the single
			// director into d.

			var d directQuery
			err = client.Unmarshal(resp.N, &d)
			if err == nil {
				log.Print("Found ", d.Root.ID, " : ", d.Root.Name)
				enqueueDirector(&d.Root)
			} else {
				log.Printf("Couldn't unmarshar response for %s.", dir)
				log.Printf(err.Error())
			}
		}
	}
}

// visitDirector is called when a director's UID is taken off the queue of
// directors to visit.  Directors only get into the queue if they haven't
// been seen, so this function is called only once for each director.
func visitDirector(dir uint64, dgraphClient *client.Dgraph) {

	if edgeCount < *edgeBound {

		directorMoviesMap["$a"] = fmt.Sprint(dir)

		resp, err := runQueryWithVariables(dgraphClient, directorsMoviesTemplate, directorMoviesMap)
		if err != nil {
			log.Printf("Error processing director : %v.\n%s", dir, err)
			return // fall over for this director, but keep trying others
		}

		if len(resp.N) == 0 {
			log.Printf("Found 0 movies for director : %v.\n", dir)
			return
		}

		// Often Unmarshal() is called at the query root to unpack the whole
		// query into a struct, but that's not always required.  Unmarshal()
		// looks through the given node and unpacks all the children's children
		// into the struct.  So we can walk through the protocol buffer response
		// and pick out the bit we want to unmarshal.  In this instance we want to get
		// all a director's movies, so we could make a struct with the director
		// and a slice of their movies, but we have no other use for that struct,
		// so we can unpack just the movies by walking into the protocol buffer
		// and unpacking the Nodes found there.
		//
		// In this case:
		//  - resp.N is a slice of Nodes for each named query, each labeled _root_
		//  - directorsMoviesTemplate has only 1 query so resp.N[0] is a Node with one child for
		//    each answer in the query
		//  - query movies asked by director UID, so there can only be one child - a node
		//    representing the director
		//  - all the directors movies are then children of that node, all labelled 'movie' because
		//    of the alias
		//
		// So if we want to unpack all the movies, we pass Unmarshal the parent Node and a structure that has a tag matching the found movies.
		var movs movieQuery
		err = client.Unmarshal(resp.N[0].Children, &movs)
		if err != nil {
			log.Print("Couldn't unmarshal response for Director ", dir)
			log.Printf(err.Error())
		}

		if len(movs.Root) > 0 {

			log.Println("Found data for director : ", dir, " - ", directors[dir].Name)

			// All the actors, characters and release dates for all this director's
			// movies have been unpacked into variable movs.
			// Tick them off so we know not to crawl them again and so we can
			// keep a count of how many edges have been found so far.
			for _, m := range movs.Root {

				// We may have already seen this movie - if it has multiple
				// directors and we've visited one of the other directors.
				if !visitedMovie(m.ID) {

					movies[m.ID] = m

					log.Println("Movie ", m.ID, " ", m.Name)

					// A movie can have a number of directors, so we'll add one edge
					// dir --- director.movie ---> m.ID
					// for each one.  We can't have previously visited any of the
					// directors (except the one we are currently visiting)
					// otherwise we would have visited this movie.
					for i := range m.Director {
						enqueueDirector(m.Director[i])
						edgeCount++
					}

					// There might be 3 edges for the languages and the release date, but we'd better check.
					if len(m.Name) > 0 {
						edgeCount++
					}
					if len(m.NameDE) > 0 {
						edgeCount++
					}
					if len(m.NameIT) > 0 {
						edgeCount++
					}
					if !m.ReleaseDate.IsZero() {
						edgeCount++
					}

					// A movie can have a number of genres.  We'll add an edge
					// m.ID --- genre ---> g.ID
					// for each one, but some might be the first time we've seen this genre, so for
					// those, we'll also have to add
					// g.ID --- name@en ---> g.name
					for _, g := range m.Genre {
						edgeCount++ // for m.ID --- genre ---> g.ID
						if !visitedGenre(g.ID) {
							visitGenre(&g)
							edgeCount++ // for g.ID --- name@en ---> g.name
						}
					}

					// Each movie has a number of performances, which record an actor playing the
					// role of a particular character in the movie.  There must be a edges
					// m.ID --- starring ---> p
					// p --- performance.actor ---> p.Actora.ID
					// p --- performance.character ---> p.Character.ID
					// But the actor may or may not have been visited so far.  If they have,
					// there's nothing else to do.  If they haven't we'll extract all the directors
					// they have worked for, add them to the director queue to be crawled and record the
					// edges we'll add for the actor.
					for _, p := range m.Starring {
						if p.Character != nil && p.Actor != nil {
							edgeCount++ // for m.ID --- starring ---> p
							edgeCount++ // for p --- perfmance.film ---> m.ID

							edgeCount++ // for p --- performance.character ---> p.Character.ID
							if _, ok := characters[p.Character.ID]; !ok {
								characters[p.Character.ID] = p.Character
								edgeCount++ // for c.ID --- name@en ---> p.Character.Name
							}

							edgeCount++ // for p --- performance.actor ---> p.Actor.ID
							edgeCount++ // for p.Actor.ID --- actor.film ---> p
							visitActor(p.Actor, dgraphClient)
						}
					}

				}

			}

		}

	}
}

// visitActor is called each time an actor is seen.  Only the first time an
// actor is seen are their movies queried from Dgraph.
func visitActor(a *actor, dgraphClient *client.Dgraph) {
	if !visitedActor(a.ID) {

		log.Println("visiting actor : ", a.ID, " - ", a.Name)

		actors[a.ID] = a
		edgeCount++ // for a.ID --- name@en ---> a.Name

		// Find all the directors this actor has worked for
		req := client.Req{}
		req.SetQuery(fmt.Sprintf(actorByIDTemplate, a.ID))
		resp, err := dgraphClient.Run(getContext(), &req)
		if err != nil {
			log.Printf("Error processing actor : %v (%v).  --- Error in getting response from server, %s", a.Name, a.ID, err)
			return
		}

		// Other queries have used the Unmarshal() function to put the query data
		// directly into a structure.  It's also possible to just grab out the results
		// directly from the protocol buffer.  In this case, we have to walk
		// deep into the query to get the result we want, so let's just grab it directly.
		//
		// - resp.N should be length 1 because there was only 1 query block
		// - resp.N[0].Children should be length 1 because the query asked by UID, so only one
		//   node can be in the answer
		if len(resp.N) == 1 && len(resp.N[0].Children) == 1 {

			// We are at an actor
			// For each performance in each movie they have acted in - actor.film
			for _, actorsMovie := range resp.N[0].Children[0].Children {

				// At a performance.
				// Really, should only be one edge to a film from a performance - performance.film
				for _, actorPerformance := range actorsMovie.Children {

					// At a film.
					// We've walked the graph response from actor to the films they have acted in.
					// If the query had more structure, we'd have to check the Atrribute name of
					// the children to check which edge we followed to get to the child.

					for _, dir := range actorPerformance.Children {

						// At a director, with a uid and a name
						var uid uint64
						var name string
						for _, prop := range dir.Properties {
							switch prop.Prop {
							case "uid":
								uid = prop.GetValue().GetUidVal()
							case "name@en":
								name = prop.GetValue().GetStrVal()
							}
						}
						enqueueDirector(&director{uid, name})
					}
				}
			}
		}
	}
}

func crawl(dgraphClient *client.Dgraph) {
	done := false

	for !done {
		select {
		case dirID := <-toVisit:
			visitDirector(dirID, dgraphClient)
		default:
			done = true
		}
	}
}

func visitGenre(g *genre) {
	genres[g.ID] = g
}

func visitedActor(actorID uint64) bool {
	_, ok := actors[actorID]
	return ok
}

func visitedMovie(movID uint64) bool {
	_, ok := movies[movID]
	return ok
}

func visitedGenre(genID uint64) bool {
	_, ok := genres[genID]
	return ok
}

func visitedDirector(dirID uint64) bool {
	_, ok := directors[dirID]
	return ok
}

func enqueueDirector(dir *director) {
	if !visitedDirector(dir.ID) {
		directors[dir.ID] = dir
		edgeCount++ // For dir.ID --- name ---> dir.Name
		go func() { toVisit <- dir.ID }()
	}
}

// ---------------------- main----------------------

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()
	if !flag.Parsed() {
		log.Fatal("Unable to parse flags")
	}

	toVisit = make(chan uint64)

	conn, err := grpc.Dial(*dgraph, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// The Dgraph client needs a directory in which to write its blank node maps.
	clientDir, err := ioutil.TempDir("", "client_")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(clientDir)

	// A DgraphClient made from a succesful connection.  All calls to the client and thus the backend go through this.
	// We won't be doing any batching with this client, just queries, so we can ignore the BatchMutationOptions.
	dgraphClient := client.NewDgraphClient([]*grpc.ClientConn{conn}, client.DefaultOptions, clientDir)
	defer dgraphClient.Close()

	// Fill up the director queue with directors
	readDirectors(directorSeeds, dgraphClient)

	// The seed directors are set up, so we can start to crawl.
	crawl(dgraphClient)

	// Write all the crawled info out to gzipped rdf.  We'll write every UID as a blank node just by prefixing every UID with _:BN.

	f, err := os.Create(*outfile)
	if err != nil {
		log.Fatalf("Couldn't open file %s", err)
	}
	defer f.Close()

	toFile := gzip.NewWriter(f)
	defer toFile.Close()

	totalEdges := 0

	// RDF triples look like
	// source-node <edge-name> target-node .
	// or
	// source-node <edge-name> scalar-value .

	toFile.Write([]byte("\n\t\t# -------- directors --------\n\n"))
	for _, dir := range directors {
		toFile.Write([]byte(fmt.Sprintf("\t\t_:BN%v <name> \"%v\"@en .\n", dir.ID, dir.Name)))
		totalEdges++
	}
	log.Printf("Wrote %v directors to %v", len(directors), *outfile)

	toFile.Write([]byte("\n\t\t# -------- actors --------\n\n"))
	for _, actor := range actors {
		toFile.Write([]byte(fmt.Sprintf("\t\t_:BN%v <name> \"%v\"@en .\n", actor.ID, actor.Name)))
		totalEdges++
	}
	log.Printf("Wrote %v actors to %v", len(actors), *outfile)

	toFile.Write([]byte("\n\t\t# -------- genres --------\n\n"))
	for _, genre := range genres {
		toFile.Write([]byte(fmt.Sprintf("\t\t_:BN%v <name> \"%v\"@en .\n", genre.ID, genre.Name)))
		totalEdges++
	}
	log.Printf("Wrote %v genres to %v", len(genres), *outfile)

	toFile.Write([]byte("\n\t\t# -------- movies --------\n\n"))

	for _, movie := range movies {
		if movie.Name != "" {
			toFile.Write([]byte(fmt.Sprintf("\t\t_:BN%v <name> \"%v\"@en .\n", movie.ID, movie.Name)))
			totalEdges++
		}
		if movie.NameDE != "" {
			toFile.Write([]byte(fmt.Sprintf("\t\t_:BN%v <name> \"%v\"@de .\n", movie.ID, movie.NameDE)))
			totalEdges++
		}
		if movie.NameIT != "" {
			toFile.Write([]byte(fmt.Sprintf("\t\t_:BN%v <name> \"%v\"@it .\n", movie.ID, movie.NameIT)))
			totalEdges++
		}

		for _, dir := range movie.Director {
			toFile.Write([]byte(fmt.Sprintf("\t\t_:BN%v <director.film> _:BN%v .\n", dir.ID, movie.ID)))
			totalEdges++
		}

		for _, genre := range movie.Genre {
			toFile.Write([]byte(fmt.Sprintf("\t\t_:BN%v <genre> _:BN%v .\n", movie.ID, genre.ID)))
			totalEdges++
		}

		toFile.Write([]byte(fmt.Sprintf("\t\t_:BN%v <initial_release_date> \"%v\" .\n", movie.ID, movie.ReleaseDate.Format(time.RFC3339))))
		totalEdges++

		for _, p := range movie.Starring {
			if p.Character != nil && p.Actor != nil {
				pBlankNode := uuid.NewV4()

				toFile.Write([]byte(fmt.Sprintf("\t\t_:BN%v <starring> _:BN%v .\n", movie.ID, pBlankNode)))
				toFile.Write([]byte(fmt.Sprintf("\t\t_:BN%v <performance.film> _:BN%v .\n", pBlankNode, movie.ID)))

				toFile.Write([]byte(fmt.Sprintf("\t\t_:BN%v <performance.actor> _:BN%v .\n", pBlankNode, p.Actor.ID)))
				toFile.Write([]byte(fmt.Sprintf("\t\t_:BN%v <actor.film> _:BN%v .\n", p.Actor.ID, pBlankNode)))

				toFile.Write([]byte(fmt.Sprintf("\t\t_:BN%v <performance.character> _:BN%v .\n", pBlankNode, p.Character.ID)))
				toFile.Write([]byte(fmt.Sprintf("\t\t_:%v <name> \"%v\"@en .\n", p.Character.ID, p.Character.Name)))
				totalEdges += 6
			}
		}
		toFile.Write([]byte(fmt.Sprintln()))
	}
	log.Printf("Wrote %v movies to %v", len(movies), *outfile)

	log.Printf("Wrote %v edges in total to %v", totalEdges, *outfile)
}
