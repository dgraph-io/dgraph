+++
title = "Clients"
+++

## Implementation

Clients communicate with the server using [Protocol Buffers](https://developers.google.com/protocol-buffers) over [gRPC](http://www.grpc.io/). This requires [defining services](http://www.grpc.io/docs/#defining-a-service) and data types in a ''proto'' file and then generating client and server side code using the [protoc compiler](https://github.com/google/protobuf).

The proto file used by Dgraph is located at [graphresponse.proto](https://github.com/dgraph-io/dgraph/blob/master/protos/graphp/graphresponse.proto).

## Languages ##
### Go ###
After you have the followed [Get started]({{< relref "get-started/index.md">}}) and got the server running on `127.0.0.1:8080`, you can use the Go client to run queries and mutations as shown in the example below.

{{% notice "note" %}}The example below would store values with the correct types only if Dgraph is run with a [schema]({{< relref "query-language/index.md#schema" >}}) defining the appropriate types, otherwise everything would be stored as a string.{{% /notice %}}

#### Installation ####

To get the Go client, you can run
```
go get -u -v github.com/dgraph-io/dgraph/client github.com/dgraph-io/dgraph/protos/graphp
```

#### Example ####

```
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/twpayne/go-geom/encoding/wkb"
)

var (
	dgraph = flag.String("d", "127.0.0.1:8080", "Dgraph server address")
)

func main() {
	conn, err := grpc.Dial(*dgraphp, grpc.WithInsecure())

	// Creating a new client.
	c := graphp.NewDgraphClient(conn)
	// Starting a new request.
	req := client.Req{}
	// _:person1 tells Dgraph to assign a new Uid and is the preferred way of creating new nodes.
	// See https://wiki.dgraph.io/index.php?title=Query_Language&redirect=no#Assigning_UID for more details.
	nq := graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "name",
	}
	// Str is a helper function to add a string value.
	client.Str("Steven Spielberg", &nq)
	// Adding a new mutation.
	req.AddMutation(nq, client.SET)

	nq = graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "now",
	}
	// Datetime is a helper function to add a datetime value.
	if err = client.Datetime(time.Now(), &nq); err != nil {
		log.Fatal(err)
	}
	// Adding another mutation to the same request.
	req.AddMutation(nq, client.SET)

	nq = graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "birthday",
	}
	// Date is a helper function to add a date value.
	if err = client.Date(time.Date(1991, 2, 1, 0, 0, 0, 0, time.UTC), &nq); err != nil {
		log.Fatal(err)
	}
	// Add another mutation.
	req.AddMutation(nq, client.SET)

	nq = graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "loc",
	}
	// ValueFromGeoJson is used to set a Geo value.
	if err = client.ValueFromGeoJson(`{"type":"Point","coordinates":[-122.2207184,37.72129059]}`, &nq); err != nil {
		log.Fatal(err)
	}
	req.AddMutation(nq, client.SET)

	nq = graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "age",
	}
	// Int is used to add integer values.
	if err = client.Int(25, &nq); err != nil {
		log.Fatal(err)
	}
	req.AddMutation(nq, client.SET)

	nq = graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "salary",
	}
	// Float is used to floating values.
	if err = client.Float(13333.6161, &nq); err != nil {
		log.Fatal(err)
	}
	req.AddMutation(nq, client.SET)

	nq = graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "married",
	}
	// Bool is used to set boolean values.
	if err = client.Bool(false, &nq); err != nil {
		log.Fatal(err)
	}
	req.AddMutation(nq, client.SET)

	// Lets create another person and add a name for it.
	nq = graphp.NQuad{
		Subject:   "_:person2",
		Predicate: "name",
	}
	// Str is a helper function to add a string value.
	client.Str("William Jones", &nq)
	// Adding a new mutation.
	req.AddMutation(nq, client.SET)

	// Lets connect the two nodes together.
	nq = graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "friend",
		ObjectId:  "_:person2",
	}
	req.AddMutation(nq, client.SET)
	// Lets run the request with all these mutations.
	resp, err := c.Run(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}
	person1Uid := resp.AssignedUids["person1"]
        person2Uid := resp.AssignedUids["person2"]

	// Lets initiate a new request and query for the data.
	req = client.Req{}
	// Lets set the starting node id to person1Uid.
	req.SetQuery(fmt.Sprintf("{ me(id: %v) { _uid_ name now birthday loc salary age married friend {_uid_ name} } }", client.Uid(person1Uid)))
	resp, err = c.Run(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}

	person1 := resp.N[0].Children[0]
	props := person1.Properties
	name := props[0].Value.GetStrVal()
	fmt.Println("Name: ", name)

	// We use time.Parse for Date and Datetime values, to get the actual value back.
	now, err := time.Parse(time.RFC3339, props[1].Value.GetStrVal())
	if err != nil {
		log.Fatalf("Error in parsing time, %s", err)
	}
	fmt.Println("Now: ", now)

	birthday, err := time.Parse(time.RFC3339, props[2].Value.GetStrVal())
	if err != nil {
		log.Fatalf("Error in parsing time, %s", err)
	}
	fmt.Println("Birthday: ", birthday)

	// We use wkb.Unmarshal to get the geom object back from Geo val.
	geom, err := wkb.Unmarshal(props[3].Value.GetGeoVal())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Loc: ", geom)

	fmt.Println("Salary: ", props[4].Value.GetDoubleVal())
	fmt.Println("Age: ", props[5].Value.GetIntVal())
	fmt.Println("Married: ", props[6].Value.GetBoolVal())

	person2 := person1.Children[0]
	fmt.Printf("%v name: %v\n", person2.Attribute, person2.Properties[0].Value.GetStrVal())

	// Deleting an edge.
	nq = graphp.NQuad{
		Subject:   client.Uid(person1Uid),
		Predicate: "friend",
		ObjectId:  client.Uid(person2Uid),
	}
	req = client.Req{}
	req.AddMutation(nq, client.DEL)
	// Lets run the request with all these mutations.
	resp, err = c.Run(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}
}
```

An appropriate schema for the above example would be
```
curl localhost:8080/query -XPOST -d $'
mutation {
  schema {
	now: datetime
	birthday: date
	age: int
	salary: float
	name: string
	loc: geo
	married: bool
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

### Python ###
{{% notice "incomplete" %}}A lot of development has gone into the Go client and the Python client is not up to date with it. We are looking for help from contributors to bring it up to date.{{% /notice %}}

#### Installation ####

##### Via pip #####

```
pip install -U pydgraph
```

##### By Source #####
* Clone the git repository for python client from github.

```
git clone https://github.com/dgraph-io/pydgraph
cd pydgraph

# Optional: If you have the dgraph server running on localhost:8080 you can run tests.
python setup.py test

# Install the python package.
python setup.py install
```

#### Example ####
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

### Java ###
{{% notice "incomplete" %}}A lot of development has gone into the Go client and the Java client is not up to date with it. We are looking for help from contributors to bring it up to date.{{% /notice %}}

#### Installation ####

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

#### Example ###
You just need to include the `fatJar` into the `classpath`, the following is a simple
example of how to use it:

* Write `DgraphMain.java` (assuming Dgraph contains the data required for the query):

```
import io.dgraph.client.DgraphClient;
import io.dgraph.client.GrpcDgraphClient;
import io.dgraph.client.DgraphResult;

public class DgraphMain {
    public static void main(final String[] args) {
        final DgraphClient dgraphClient = GrpcDgraphClient.newInstance("localhost", 8080);
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
INFO: [ManagedChannelImpl@5d3411d] Created with target localhost:8080
{"_root_":[{"_uid_":"0x8c84811dffd0a905","_xid_":"alice","name":"Alice","follows":[{"_uid_":"0xdd77c65008e3c71","_xid_":"bob","name":"Bob"},{"_uid_":"0x5991e7d8205041b3","_xid_":"greg","name":"Greg"}]}],"server_latency":{"pb":"11.487µs","parsing":"85.504µs","processing":"270.597µs"}}
```

### Shell ###

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

#### Example ###

This example, from [Get Started]({{< relref ="get-started/index.md" >}}), [Movies by Steven Spielberg]({{< relref "get-started/index.md#movies-by-steven-spielberg" >}}), uses commands commonly available on a Dgraph server to query the local Dgraph server.

```
curl localhost:8080/query -sS -XPOST -d $'{
  director(allof("name", "steven spielberg")) {
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
  director(allof("name", "steven spielberg")) {
    name@en
    director.film (orderdesc: initial_release_date) {
      name@en
      initial_release_date
    }
  }
}
' | python3 -m json.tool | more
```