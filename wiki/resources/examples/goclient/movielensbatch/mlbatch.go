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

Go routines concurrently read from text input files and batch writes using the go client.

Another version of this program outputed gzipped RDF
https://github.com/dgraph-io/benchmarks/tree/master/movielens/conv100k
It's output was used in blog posts about building recommendation systems in Dgraph
https://open.dgraph.io/post/recommendation/
https://open.dgraph.io/post/recommendation2/


To run the program:

- build with

go build mlbatch.go

- start a dgraph instance, e.g for default options

dgraph 

- run in a directory with the movielens data unpacked

./mlbatch

Use --help on dgraph or mlbatch to see the options - dgraph's --grpc_port must match mlbatch's --d
and the unpacked movieLens data must be in dir ./ml-100k or --genre, etc given as options.

You can add a schema with

mutation {
  schema {
    name: string @index(term, exact) .
	rated: uid @reverse @count .
	genre: uid @reverse .
  }
}

and run queries like

{
  q(func: eq(name, "Toy Story (1995)")) {
    name
    genre { 
      name 
    }
    count(~rated) 
  }
}

or see the queries at
https://open.dgraph.io/post/recommendation/   and 
https://open.dgraph.io/post/recommendation2/
(though your UIDs will be different)



*** Check out the blog post about this code : https://open.dgraph.io/post/client0.8.0 ***

*/

package main

import (
	"bufio"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/x"
)

var (
	dgraph     = flag.String("d", "127.0.0.1:9080", "Dgraph gRPC server address")
	concurrent = flag.Int("c", 10, "Number of concurrent requests to make to Dgraph")
	numRdf     = flag.Int("m", 100, "Number of RDF N-Quads to send as part of a mutation.")

	genre = flag.String("genre", "ml-100k/u.genre", "")
	data = flag.String("rating", "ml-100k/u.data", "")
	users  = flag.String("user", "ml-100k/u.user", "")
	movie = flag.String("movie", "ml-100k/u.item", "")
)

func main() {

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()
	if !flag.Parsed() {
		log.Fatal("Unable to parse flags")
	}

	conn, err := grpc.Dial(*dgraph, grpc.WithInsecure())
	x.Check(err)
	defer conn.Close()

	// The Dgraph client needs a directory in which to write its blank node maps.
	// We'll use blank nodes below to record users and movies, when we need them again
	// we can look them up in the client's map and get the right node without having
	// to record or issue a query.
	//
	// The blank node map records blank-node-name -> Node, so everytime in the load we refer
	// to a node we can grab it from the map.  These names aren't presisted in Dgraph (if we
	// wanted that we'd use an XID).
	clientDir, err := ioutil.TempDir("", "client_")
	x.Check(err)
	defer os.RemoveAll(clientDir)

	// The client builds Pending concurrent batchs each of size Size and submits to the server
	// as each batch fills.
	bmOpts := client.BatchMutationOptions{
		Size:          *numRdf,
		Pending:       *concurrent,
		PrintCounters: true,
	}
	dgraphClient := client.NewDgraphClient([]*grpc.ClientConn{conn}, bmOpts, clientDir)
	defer dgraphClient.Close()

	// We aren't writing any schema here.  You can load the schema at
	// https://github.com/dgraph-io/benchmarks/blob/master/movielens/movielens.schema
	// using draphloader (--s option with no data file), or by running a mutation
	// mutation { schema { <content of schema file> } }
	// dgraphloader function processSchemaFile() also shows an example of how to load
	// a schema file using the client.

	gf, err := os.Open(*genre)
	x.Check(err)
	defer gf.Close()
	uf, err := os.Open(*users)
	x.Check(err)
	defer gf.Close()
	df, err := os.Open(*data)
	x.Check(err)
	defer df.Close()
	mf, err := os.Open(*movie)
	x.Check(err)
	defer mf.Close()

	var wg sync.WaitGroup
	wg.Add(4)

	// The blank node names are used accross 4 go routines.  It doesn't mater which go routine
	// uses the node name first - a "user<userID>" node may be first used in the go routine reading
	// users, or the one setting ratings - the Dgraph client ensures that all the routines are
	// talking about the same nodes. So no matter what the creation order, or the order the client 
	// submits the batches in the background, the graph is connected properly.

	go func() {
		defer wg.Done()

		// Each line in genre file looks like
		// genre-name|genreID
		// We'll use a blank node named "genre<genreID>" to identify each genre node
		br := bufio.NewReader(gf)
		log.Println("Reading genre file")
		for {
			line, err := br.ReadString('\n')
			if err != nil && err == io.EOF {
				break
			}
			line = strings.Trim(line, "\n")
			csv := strings.Split(line, "|")
			if len(csv) != 2 {
				continue
			}

			n, err := dgraphClient.NodeBlank("genre" + csv[1])
			x.Check(err)

			e := n.Edge("name")
			x.Check(e.SetValueString(csv[0]))
			x.Check(dgraphClient.BatchSet(e))
		}
	}()

	go func() {
		defer wg.Done()

		// Each line in the user file looks like
		// userID|age|genre|occupation|ZIPcode
		// We'll use a blank node named "user<userID>" to identify each user node
		br := bufio.NewReader(uf)
		log.Println("Reading user file")
		for {
			line, err := br.ReadString('\n')
			if err != nil && err == io.EOF {
				break
			}
			line = strings.Trim(line, "\n")
			csv := strings.Split(line, "|")
			if len(csv) != 5 {
				continue
			}

			n, err := dgraphClient.NodeBlank("user" + csv[0])
			x.Check(err)

			e := n.Edge("age")
			age, err := strconv.ParseInt(csv[1], 10, 32)
			x.Check(err)
			x.Check(e.SetValueInt(age))
			x.Check(dgraphClient.BatchSet(e))

			e = n.Edge("gender")
			x.Check(e.SetValueString(csv[2]))
			x.Check(dgraphClient.BatchSet(e))

			e = n.Edge("occupation")
			x.Check(e.SetValueString(csv[3]))
			x.Check(dgraphClient.BatchSet(e))

			e = n.Edge("zipcode")
			x.Check(e.SetValueString(csv[4]))
			x.Check(dgraphClient.BatchSet(e))
		}
	}()

	go func() {
		defer wg.Done()

		// The lines of the movie file look like
		// movieID|movie-name|date||imdb-address|genre0?|genre1?|...|genre18?
		// We'll use "movie<movieID>" as the blank node name
		br := bufio.NewReader(mf)
		log.Println("Reading movies file")
		for {
			line, err := br.ReadString('\n')
			if err != nil && err == io.EOF {
				break
			}
			line = strings.Trim(line, "\n")
			csv := strings.Split(line, "|")
			if len(csv) != 24 {
				continue
			}

			n, err := dgraphClient.NodeBlank("movie" + csv[0])
			x.Check(err)

			e := n.Edge("name")
			x.Check(e.SetValueString(csv[1]))
			x.Check(dgraphClient.BatchSet(e))

			// 1 means the movie has the corresponding genre
			for i := 5; i < 24; i++ {
				if csv[i] == "0" {
					continue
				}

				g, err := dgraphClient.NodeBlank("genre" + strconv.Itoa(i-5))
				x.Check(err)
				e = n.ConnectTo("genre", g)
				x.Check(dgraphClient.BatchSet(e))
			}
		}
	}()

	go func() {
		defer wg.Done()

		// Each line in the rating file looks like
		// userID     movieID     rating       timestamp
		br := bufio.NewReader(df)
		log.Println("Reading rating file")
		for {
			line, err := br.ReadString('\n')
			if err != nil && err == io.EOF {
				break
			}
			line = strings.Trim(line, "\n")
			csv := strings.Split(line, "\t")
			if len(csv) != 4 {
				continue
			}

			u, err := dgraphClient.NodeBlank("user" + csv[0])
			x.Check(err)

			m, err := dgraphClient.NodeBlank("movie" + csv[1])
			x.Check(err)

			e := u.ConnectTo("rated", m)
			e.AddFacet("rating", csv[2])
			x.Check(dgraphClient.BatchSet(e))
		}
	}()

	wg.Wait()

	// This must be called at the end of a batch to ensure all batched mutations are sent to the store.
	dgraphClient.BatchFlush()

	log.Println("Finised.")
}
