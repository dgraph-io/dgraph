+++
date = "2017-03-20T19:35:35+11:00"
title = "Clients"
+++

## Implementation

All clients can communicate with the server via the HTTP endpoint (set with option `--port` when starting Dgraph).  Queries and mutations can be submitted and JSON is returned.

Go clients can use the clients package and communicate with the server over [gRPC](http://www.grpc.io/).  Internally this uses [Protocol Buffers](https://developers.google.com/protocol-buffers) and the proto file used by Dgraph is located at [graphresponse.proto](https://github.com/dgraph-io/dgraph/blob/master/protos/graphresponse.proto).


## Languages

### Go

[![GoDoc](https://godoc.org/github.com/dgraph-io/dgraph/client?status.svg)](https://godoc.org/github.com/dgraph-io/dgraph/client)

The go client communicates with the server on the grpc port (set with option `--grpc_port` when starting Dgraph).


#### Installation

Go get the client:
```
go get -u -v github.com/dgraph-io/dgraph/client
```

#### Examples

The client [GoDoc](https://godoc.org/github.com/dgraph-io/dgraph/client) has specifications of all functions and examples.

Larger examples can be found [here](https://github.com/dgraph-io/dgraph/tree/master/wiki/resources/examples/goclient).  And [this](https://open.dgraph.io/post/client0.8.0) blog post explores the examples further.

The app [dgraphloader](https://github.com/dgraph-io/dgraph/tree/master/cmd/dgraphloader) uses the client interface to batch concurrent mutations.

{{% notice "note" %}}As with mutations through a mutation block, [schema type]({{< relref "query-language/index.md#schema" >}}) needs to be set for the edges, or schema is derived based on first mutation received by the server. {{% /notice %}}

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
