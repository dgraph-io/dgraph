+++
title = "Get Started - Quickstart Guide"
aliases = ["/get-started-old"]
[menu.main]
  url = "/get-started"
  name = "Get Started"
  identifier = "get-started"
  parent = "dql"
  weight = 2
+++

{{% notice "note" %}}
This is a quick start guide.
You can find the getting started tutorial series [here]({{< relref "tutorials/index.md" >}}).
{{% /notice %}}

## Dgraph

Designed from the ground up to be run in production, **Dgraph** is the native GraphQL database with a graph backend. It is open-source, scalable, distributed, highly available and lightning fast.


Dgraph cluster consists of different nodes (Zero, Alpha & Ratel), and each node serves a
different purpose.

- **Dgraph Zero** controls the Dgraph cluster, assigns servers to a group,
and re-balances data between server groups.

- **Dgraph Alpha** hosts predicates and indexes. Predicates are either the properties
associated with a node or the relationship between two nodes. Indexes are the tokenizers
that can be associated with the predicates to enable filtering using appropriate functions.

- **Ratel** serves the UI to run queries, mutations & altering schema.

You need at least one Dgraph Zero and one Dgraph Alpha to get started.

**Here's a four-step tutorial to get you up and running.**

This is a quick-start guide to running Dgraph.
For an interactive walkthrough, take the [tour](https://dgraph.io/tour/).

{{% notice "tip" %}}
This guide is for the powerful query language of Dgraph, [GraphQL+-](https://dgraph.io/docs/master/query-language/#graphql)
which is a variation of a query language created by Facebook, [GraphQL](https://graphql.org/).

You can find the instructions to get started with GraphQL from
[dgraph.io/graphql](https://dgraph.io/graphql).
{{% /notice %}}

### Step 1: Run Dgraph

There are several ways to install and run Dgraph, all of which
you can find in the [Download page](https://dgraph.io/downloads)

The easiest way to get Dgraph up and running is using the `dgraph/standalone` docker image.
Follow the instructions [here](https://docs.docker.com/install) to install
Docker if you don't have it already.

_This standalone image is meant for quickstart purposes only.
It is not recommended for production environments._

```sh
docker run --rm -it -p 8080:8080 -p 9080:9080 -p 8000:8000 -v ~/dgraph:/dgraph dgraph/standalone:{{< version >}}
```

This would start a single container with **Dgraph Alpha**, **Dgraph Zero** and **Ratel** running in it.
You would find the Dgraph data stored in a folder named *dgraph* of your *home directory*.

{{% notice "tip" %}}
Usually, you need to set the estimated memory Dgraph alpha can take through `lru_mb` flag.
This is just a hint to the Dgraph alpha, and actual usage would be higher than this.
It is recommended to set lru_mb to the one-third of the available RAM. For the standalone setup,
it is set to that by default.
{{% /notice %}}


### Step 2: Run Mutation

{{% notice "tip" %}}
Once Dgraph is running, you can access **Ratel** at [`http://localhost:8000`](http://localhost:8000).
It allows browser-based queries, mutations and visualizations.

You can run the mutations and queries below from either curl in the command line
or by pasting the mutation data in the **Ratel**.
{{% /notice %}}

#### Dataset
The dataset is a movie graph, where and the graph nodes are
entities of the type directors, actors, genres, or movies.

#### Storing data in the graph
Changing the data stored in Dgraph is a mutation. Dgraph as of now supports
mutation for two kinds of data: RDF and JSON. The following RDF mutation
stores information about the first three releases of the the ''Star Wars''
series and one of the ''Star Trek'' movies. Running the RDF mutation, either
through the curl or Ratel UI's mutate tab will store the data in Dgraph.

```sh
curl -H "Content-Type: application/rdf" "localhost:8080/mutate?commitNow=true" -XPOST -d $'
{
  set {
   _:luke <name> "Luke Skywalker" .
   _:luke <dgraph.type> "Person" .
   _:leia <name> "Princess Leia" .
   _:leia <dgraph.type> "Person" .
   _:han <name> "Han Solo" .
   _:han <dgraph.type> "Person" .
   _:lucas <name> "George Lucas" .
   _:lucas <dgraph.type> "Person" .
   _:irvin <name> "Irvin Kernshner" .
   _:irvin <dgraph.type> "Person" .
   _:richard <name> "Richard Marquand" .
   _:richard <dgraph.type> "Person" .

   _:sw1 <name> "Star Wars: Episode IV - A New Hope" .
   _:sw1 <release_date> "1977-05-25" .
   _:sw1 <revenue> "775000000" .
   _:sw1 <running_time> "121" .
   _:sw1 <starring> _:luke .
   _:sw1 <starring> _:leia .
   _:sw1 <starring> _:han .
   _:sw1 <director> _:lucas .
   _:sw1 <dgraph.type> "Film" .

   _:sw2 <name> "Star Wars: Episode V - The Empire Strikes Back" .
   _:sw2 <release_date> "1980-05-21" .
   _:sw2 <revenue> "534000000" .
   _:sw2 <running_time> "124" .
   _:sw2 <starring> _:luke .
   _:sw2 <starring> _:leia .
   _:sw2 <starring> _:han .
   _:sw2 <director> _:irvin .
   _:sw2 <dgraph.type> "Film" .

   _:sw3 <name> "Star Wars: Episode VI - Return of the Jedi" .
   _:sw3 <release_date> "1983-05-25" .
   _:sw3 <revenue> "572000000" .
   _:sw3 <running_time> "131" .
   _:sw3 <starring> _:luke .
   _:sw3 <starring> _:leia .
   _:sw3 <starring> _:han .
   _:sw3 <director> _:richard .
   _:sw3 <dgraph.type> "Film" .

   _:st1 <name> "Star Trek: The Motion Picture" .
   _:st1 <release_date> "1979-12-07" .
   _:st1 <revenue> "139000000" .
   _:st1 <running_time> "132" .
   _:st1 <dgraph.type> "Film" .
  }
}
' | python -m json.tool | less
```


{{% notice "tip" %}}
To run an RDF/JSON mutation using a file via curl, you can use the curl option
`--data-binary @/path/to/mutation.rdf` instead of `-d $''`.
The `--data-binary` option skips curl's default URL-encoding.
{{% /notice %}}

### Step 3: Alter Schema

Alter the schema to add indexes on some of the data so queries can use term matching, filtering and sorting.

```sh
curl "localhost:8080/alter" -XPOST -d $'
  name: string @index(term) .
  release_date: datetime @index(year) .
  revenue: float .
  running_time: int .

  type Person {
    name
  }

  type Film {
    name
    release_date
    revenue
    running_time
    starring
    director
  }
' | python -m json.tool | less
```

{{% notice "tip" %}}
To submit the schema from the Ratel UI, go to Schema page,
click on **Bulk Edit**, and paste the schema.
{{% /notice %}}

### Step 4: Run Queries

#### Get all movies
Run this query to get all the movies.
The query lists all the movies that have a starring edge.

```sh
curl -H "Content-Type: application/graphql+-" "localhost:8080/query" -XPOST -d $'
{
 me(func: has(starring)) {
   name
  }
}
' | python -m json.tool | less
```

{{% notice "tip" %}}
You can also run the GraphQL+- query from the Ratel UI's query tab.
{{% /notice %}}

#### Get all movies released after "1980"
Run this query to get "Star Wars" movies released after "1980".
Try it in the user interface to see the result as a graph.

```sh
curl -H "Content-Type: application/graphql+-" "localhost:8080/query" -XPOST -d $'
{
  me(func:allofterms(name, "Star Wars")) @filter(ge(release_date, "1980")) {
    name
    release_date
    revenue
    running_time
    director {
     name
    }
    starring {
     name
    }
  }
}
' | python -m json.tool | less
```

Output:

```json
{
  "data":{
    "me":[
      {
        "name":"Star Wars: Episode V - The Empire Strikes Back",
        "release_date":"1980-05-21T00:00:00Z",
        "revenue":534000000.0,
        "running_time":124,
        "director":[
          {
            "name":"Irvin Kernshner"
          }
        ],
        "starring":[
          {
            "name":"Han Solo"
          },
          {
            "name":"Luke Skywalker"
          },
          {
            "name":"Princess Leia"
          }
        ]
      },
      {
        "name":"Star Wars: Episode VI - Return of the Jedi",
        "release_date":"1983-05-25T00:00:00Z",
        "revenue":572000000.0,
        "running_time":131,
        "director":[
          {
            "name":"Richard Marquand"
          }
        ],
        "starring":[
          {
            "name":"Han Solo"
          },
          {
            "name":"Luke Skywalker"
          },
          {
            "name":"Princess Leia"
          }
        ]
      }
    ]
  }
}
```

That's it! In these four steps, we set up Dgraph, added some
data, set a schema and queried that data back.

## Where to go from here

- Go to [Clients]({{< relref "clients/_index.md" >}}) to see how to
communicate with Dgraph from your application.
- Take the [Tour](https://dgraph.io/tour/) for a guided tour of how to write queries in Dgraph.
- A wider range of queries can also be found in the
[Query Language]({{< relref "query-language/_index.md" >}}) reference.
- See [Deploy]({{< relref "deploy/_index.md" >}}) if you wish to run Dgraph
  in a cluster.

## Need Help

* Please use [discuss.dgraph.io](https://discuss.dgraph.io) for questions,
feature requests and discussions.
* Please use [Github Issues](https://github.com/dgraph-io/dgraph/issues)
if you encounter bugs or have feature requests.
