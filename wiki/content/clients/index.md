+++
date = "2017-03-20T19:35:35+11:00"
title = "Clients"
+++

## Implementation

Clients communicate with the server using [Protocol Buffers](https://developers.google.com/protocol-buffers) over [gRPC](http://www.grpc.io/). This requires [defining services](http://www.grpc.io/docs/#defining-a-service) and data types in a ''proto'' file and then generating client and server side code using the [protoc compiler](https://github.com/google/protobuf).

The proto file used by Dgraph is located at [graphresponse.proto](https://github.com/dgraph-io/dgraph/blob/master/protos/graphp/graphresponse.proto).

## Languages

### Go

[![GoDoc](https://godoc.org/github.com/dgraph-io/dgraph/client?status.svg)](https://godoc.org/github.com/dgraph-io/dgraph/client)

After you have the followed [Get started]({{< relref "get-started/index.md">}}) and got the server running on `127.0.0.1:8080` and (for gRPC) `127.0.0.1:9080`, you can use the Go client to run queries and mutations as shown in the example below.

{{% notice "note" %}}The example below would store values with the correct types only if the
correct [schema type]({{< relref "query-language/index.md#schema" >}}) is specified in the mutation, otherwise everything would be converted to schema type and stored. Schema is derived based on first mutation received by the server. If first mutation is of default type then everything would be stored as default type.{{% /notice %}}


#### Installation

To get the Go client, you can run
```
go get -u -v github.com/dgraph-io/dgraph/client github.com/dgraph-io/dgraph/protos
```

#### Example

The working example below shows the common operations that one would perform on a graph.

```
package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

var (
	dgraph = flag.String("d", "127.0.0.1:9080", "Dgraph server address")
)

type nameFacets struct {
	Since time.Time `dgraph:"since"`
	Alias string    `dgraph:"alias"`
}

type friendFacets struct {
	Close bool `dgraph:"close"`
}

type Person struct {
	Name         string         `dgraph:"name"`
	NameFacets   nameFacets     `dgraph:"name@facets"`
	Birthday     time.Time      `dgraph:"birthday"`
	Location     []byte         `dgraph:"loc"`
	Salary       float64        `dgraph:"salary"`
	Age          int            `dgraph:"age"`
	Married      bool           `dgraph:"married"`
	ByteDob      []byte         `dgraph:"bytedob"`
	Friends      []Person       `dgraph:"friend"`
	FriendFacets []friendFacets `dgraph:"@facets"`
}

type Res struct {
	Root Person `dgraph:"me"`
}

func main() {
	conn, err := grpc.Dial(*dgraph, grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dgraphClient := client.NewDgraphClient([]*grpc.ClientConn{conn}, client.DefaultOptions)

	req := client.Req{}
	person1, err := dgraphClient.NodeBlank("person1")
	x.Check(err)

	// Creating a person node, and adding a name attribute to it.
	e := person1.Edge("name")
	err = e.SetValueString("Steven Spielberg")
	x.Check(err)
	e.AddFacet("since", "2006-01-02T15:04:05")
	e.AddFacet("alias", `"Steve"`)
	req.Set(e)

	e = person1.Edge("birthday")
	err = e.SetValueDatetime(time.Date(1991, 2, 1, 0, 0, 0, 0, time.UTC))
	x.Check(err)
	req.Set(e)

	e = person1.Edge("loc")
	err = e.SetValueGeoJson(`{"type":"Point","coordinates":[-122.2207184,37.72129059]}`)
	x.Check(err)
	req.Set(e)

	e = person1.Edge("salary")
	err = e.SetValueFloat(13333.6161)
	x.Check(err)
	req.Set(e)

	e = person1.Edge("age")
	err = e.SetValueInt(25)
	x.Check(err)
	req.Set(e)

	e = person1.Edge("married")
	err = e.SetValueBool(true)
	x.Check(err)
	req.Set(e)

	e = person1.Edge("bytedob")
	err = e.SetValueBytes([]byte("01-02-1991"))
	x.Check(err)
	req.Set(e)

	person2, err := dgraphClient.NodeBlank("person2")
	x.Check(err)

	e = person2.Edge("name")
	err = e.SetValueString("William Jones")
	x.Check(err)
	req.Set(e)

	e = person1.Edge("friend")
	err = e.ConnectTo(person2)
	x.Check(err)
	e.AddFacet("close", "true")
	req.Set(e)

	resp, err := dgraphClient.Run(context.Background(), &req)
	x.Check(err)

	req = client.Req{}
	req.SetQuery(fmt.Sprintf(`{
		me(func: uid(%v)) {
			_uid_
			name @facets
			now
			birthday
			loc
			salary
			age
			married
			bytedob
			friend @facets {
				_uid_
				name
			}
		}
	}`, person1))
	resp, err = dgraphClient.Run(context.Background(), &req)
	x.Check(err)

	var r Res
	err = client.Unmarshal(resp.N, &r)
	fmt.Printf("Steven: %+v\n\n", r.Root)
	fmt.Printf("William: %+v\n", r.Root.Friends[0])

	req = client.Req{}
	// Here is an example of deleting an edge
	e = person1.Edge("friend")
	err = e.ConnectTo(person2)
	x.Check(err)
	req.Delete(e)
	resp, err = dgraphClient.Run(context.Background(), &req)
	x.Check(err)
}
```

{{% notice "note" %}}Type for the facets are automatically interpreted from the value. If you want it to
be interpreted as string, it has to be a raw string literal with `""` as shown above for `alias` facet.{{% /notice %}}

An appropriate schema for the above example would be
```
curl localhost:8080/query -XPOST -d $'
mutation {
  schema {
	now: dateTime .
	birthday: date .
	age: int .
	salary: float .
	name: string .
	loc: geo .
	married: bool .
  }
}' | python -m json.tool | less
```

Apart from the above syntax, you could perform all the queries and mutations listed on [Query Language]({{< relref "query-language/index.md" >}}). For example you could do this.

```
req := client.Req{}
req.SetQuery(`
mutation {
 set {
   <class1> <student> _:x .
   <class1> <name> "awesome class" .
   _:x <name> "alice" .
   _:x <planet> "Mars" .
   _:x <friend> _:y .
   _:y <name> "bob" .
 }
}
{
 class(id:class1) {
  name
  student {
   name
   planet
   friend {
    name
   }
  }
 }
}`)
resp, err := c.Run(context.Background(), req.Request())
if err != nil {
	log.Fatalf("Error in getting response from server, %s", err)
}
fmt.Printf("%+v\n", resp)
```

Here is an example of how you could use the Dgraph client to do batch mutations concurrently.
This is what the **dgraphloader** uses internally.
```
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"os"

	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/x"
)

func ExampleBatchMutation() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	// Start a new batch with batch size 1000 and 100 concurrent requests.
	bmOpts := client.BatchMutationOptions{
		Size:          1000,
		Pending:       100,
		PrintCounters: false,
	}
	dgraphClient := client.NewDgraphClient(conn, bmOpts)

	// Process your file, convert data to a protos.NQuad and add it to the batch.
	// For each edge, do a BatchSet (this would typically be done in a loop
	// after processing the data into edges). Here we show example of reading a
	// file with RDF data, converting it to edges and adding it to the batch.

	f, err := os.Open("goldendata.rdf.gz")
	x.Check(err)
	defer f.Close()
	gr, err := gzip.NewReader(f)
	x.Check(err)

	var buf bytes.Buffer
	bufReader := bufio.NewReader(gr)
	var line int
	for {
		err = x.ReadLine(bufReader, &buf)
		if err != nil {
			break
		}
		line++
		nq, err := rdf.Parse(buf.String())
		if err == rdf.ErrEmpty { // special case: comment/empty line
			buf.Reset()
			continue
		} else if err != nil {
			log.Fatalf("Error while parsing RDF: %v, on line:%v %v", err, line, buf.String())
		}
		buf.Reset()

		nq.Subject = Node(nq.Subject, dgraphClient)
		if len(nq.ObjectId) > 0 {
			nq.ObjectId = Node(nq.ObjectId, dgraphClient)
		}
		if err = dgraphClient.BatchSet(client.NewEdge(nq)); err != nil {
			log.Fatal("While adding mutation to batch: ", err)
		}
	}
	if err != io.EOF {
		x.Checkf(err, "Error while reading file")
	}
	// Wait for all requests to complete. This is very important, else some
	// data might not be sent to server.
	dgraphClient.BatchFlush()
}
```

### Python
{{% notice "incomplete" %}}A lot of development has gone into the Go client and the Python client is not up to date with it. We are looking for help from contributors to bring it up to date.{{% /notice %}}

#### Installation

##### Via pip

```
pip install -U pydgraph
```

##### By Source
* Clone the git repository for python client from github.

```
git clone https://github.com/dgraph-io/pydgraph
cd pydgraph

# Optional: If you have the dgraph server running on localhost:8080 and localhost:9080 you can run tests.
python setup.py test

# Install the python package.
python setup.py install
```

#### Example
```
In [1]: from pydgraph.client import DgraphClient
In [2]: dg_client = DgraphClient('localhost', 8080)
In [3]: response = dg_client.query("""
        mutation
        {
            set
            {
                <alice> <name> \"Alice\" .
                <greg> <name> \"Greg\" .
                <alice> <follows> <greg> .
            }
        }

        query
        {
            me(_xid_: alice)
            {
                follows
                {
                    name _xid_
                }
            }
        }
        """)
In [4]: print response
n {
  uid: 10125359828081617157
  attribute: "_root_"
  children {
    uid: 6454194656439714227
    xid: "greg"
    attribute: "follows"
    properties {
      prop: "name"
      val: "Greg"
    }
  }
}
l {
  parsing: "8.64014ms"
  processing: "302.099\302\265s"
  pb: "10.422\302\265s"
}
```

### Java
{{% notice "incomplete" %}}A lot of development has gone into the Go client and the Java client is not up to date with it. We are looking for help from contributors to bring it up to date.{{% /notice %}}

#### Installation

Currently, given that this is the first version, the distribution is done via a `fatJar`
built locally. The procedure to build it is:

```
# Get code from Github
git clone git@github.com:dgraph-io/dgraph4j.git
cd dgraph4j

# Build fatJar, from the repository
./gradlew fatJar

# Copy fatJar to your CLASSPATH were it will be included
cp dgraph4j/build/libs/dgraph4j-all-0.0.1.jar $CLASSPATH
```

#### Example
You just need to include the `fatJar` into the `classpath`, the following is a simple
example of how to use it:

* Write `DgraphMain.java` (assuming Dgraph contains the data required for the query):

```
import io.dgraph.client.DgraphClient;
import io.dgraph.client.GrpcDgraphClient;
import io.dgraph.client.DgraphResult;

public class DgraphMain {
    public static void main(final String[] args) {
        final DgraphClient dgraphClient = GrpcDgraphClient.newInstance("localhost", 9080);
        final DgraphResult result = dgraphClient.query("{me(_xid_: alice) { name _xid_ follows { name _xid_ follows {name _xid_ } } }}");
        System.out.println(result.toJsonObject().toString());
    }
}
```

* Compile:
```
javac -cp dgraph4j/build/libs/dgraph4j-all-0.0.1.jar DgraphMain.java
```

* Run:
```
java -cp dgraph4j/build/libs/dgraph4j-all-0.0.1.jar:. DgraphMain
Jun 29, 2016 12:28:03 AM io.grpc.internal.ManagedChannelImpl <init>
INFO: [ManagedChannelImpl@5d3411d] Created with target localhost:9080
{"_root_":[{"_uid_":"0x8c84811dffd0a905","_xid_":"alice","name":"Alice","follows":[{"_uid_":"0xdd77c65008e3c71","_xid_":"bob","name":"Bob"},{"_uid_":"0x5991e7d8205041b3","_xid_":"greg","name":"Greg"}]}],"server_latency":{"pb":"11.487µs","parsing":"85.504µs","processing":"270.597µs"}}
```

### Shell

This client requires commands which are often already installed on a Dgraph server or a client machine.

Verify that your installation has the required commands.
```
curl   -V 2>/dev/null || wget    -V
python -V 2>/dev/null || python3 -V
less   -V 2>/dev/null || more    -V
```

Your first choices are `curl`, `python` and `less`, however you can substitute the alternatives:
`wget` for `curl`,
`python3` for `python`,
and `more` for `less`.

If you are missing both of a pair of alternates, you will need to install one of them. If both are available, you may use either.

Notice that we wrap the query text in `$'…'`. This preserves newlines in the quoted text.
This is not strictly necessary for queries, but is required by the RDF format used with mutates.

The `json.tool` module is part of the standard release package for python and python3.

#### Example

This example, from [Get Started]({{< relref "get-started/index.md" >}}), [Movies by Steven Spielberg]({{< relref "get-started/index.md#movies-by-steven-spielberg" >}}), uses commands commonly available on a Dgraph server to query the local Dgraph server.

```
curl localhost:8080/query -sS -XPOST -d $'{
  director(allofterms("name", "steven spielberg")) {
    name@en
    director.film (orderdesc: initial_release_date) {
      name@en
      initial_release_date
    }
  }
}' | python -m json.tool | less
```
And here using all alternatives.

```
wget localhost:8080/query -q -O- --post-data=$'{
  director(allofterms("name", "steven spielberg")) {
    name@en
    director.film (orderdesc: initial_release_date) {
      name@en
      initial_release_date
    }
  }
}
' | python3 -m json.tool | more
```
