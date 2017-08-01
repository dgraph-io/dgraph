+++
title = "Get Started"
+++

**New to Dgraph? Here's a 3 step tutorial to get you up and running.**

This is a quick-start guide to running Dgraph. For an interactive walk through, take the [tour](https://tour.dgraph.io).

You can see the accompanying [video here](https://www.youtube.com/watch?v=QIIdSp2zLcs).
## Step 1: Install Dgraph

Dgraph can be installed from the install scripts, or deployed in Docker.

{{% notice "note" %}}These instructions will install the latest release version.  To instead install our nightly build see [these instructions]({{< relref "deploy/index.md#nightly" >}}).{{% /notice %}}

### From Install Scripts

Install the binaries with

```sh
curl https://get.dgraph.io -sSf | bash
```

The script automatically installs Dgraph. Once done, jump straight to [step 2]({{< relref "#step-2-run-dgraph" >}}).

**Alternative:** To mitigate potential security risks, instead try:

```sh
curl https://get.dgraph.io > /tmp/get.sh
vim /tmp/get.sh  # Inspect the script
sh /tmp/get.sh   # Execute the script
```

### From Docker Image

Pull the Dgraph Docker images [from here](https://hub.docker.com/r/dgraph/dgraph/). From a terminal:

```sh
docker pull dgraph/dgraph
```

## Step 2: Run Dgraph

### From Installed Binary
If Dgraph was installed with the install script, run Dgraph with:

```sh
dgraph
```

### Using Docker

The `-v` flag lets Docker mount a directory so that dgraph can persist data to disk and access files for loading data.

#### Map to default ports (8080 and 9080 on the local interface)

```sh
mkdir -p ~/dgraph
docker run -it -p 127.0.0.1:8080:8080 -p 127.0.0.1:9080:9080 -v ~/dgraph:/dgraph --name dgraph dgraph/dgraph dgraph --bindall=true
```

#### Map to custom port
```sh
mkdir -p ~/dgraph
# Mapping port 8080 from within the container to 18080 (bound to the local interface) of the instance, likewise with the gRPC port 9090.
docker run -it -p 127.0.0.1:18080:8080 -p 127.0.0.1:19090:9090 -v ~/dgraph:/dgraph --name dgraph dgraph/dgraph dgraph --bindall=true
```

{{% notice "note" %}}The dgraph server listens on ports 8080 and 9090 (unless mapped to another port above) with log output to the terminal.{{% /notice %}}

## Step 3: Run Queries
{{% notice "tip" %}}Once Dgraph is running, a user interface is available at [`http://localhost:8080`](http://localhost:8080).  It allows browser-based queries, mutations and visualizations.

The mutations and queries below can either be run from the command line using `curl localhost:8080/query -XPOST -d $'...'` or by pasting everything between the two `'` into the running user interface on localhost.{{% /notice %}}


Changing the data or schema stored in Dgraph is a mutation.  The following mutation stores information about the first three releases of the the ''Star Wars'' series and one of the ''Star Trek'' movies.  Running this mutation, either through the UI or on the command line, will store the data in Dgraph.


```sh
curl localhost:8080/query -XPOST -d $'
mutation {
  set {
   _:luke <name> "Luke Skywalker" .
   _:leia <name> "Princess Leia" .
   _:han <name> "Han Solo" .
   _:lucas <name> "George Lucas" .
   _:irvin <name> "Irvin Kernshner" .
   _:richard <name> "Richard Marquand" .

   _:sw1 <name> "Star Wars: Episode IV - A New Hope" .
   _:sw1 <release_date> "1977-05-25" .
   _:sw1 <revenue> "775000000" .
   _:sw1 <running_time> "121" .
   _:sw1 <starring> _:luke .
   _:sw1 <starring> _:leia .
   _:sw1 <starring> _:han .
   _:sw1 <director> _:lucas .

   _:sw2 <name> "Star Wars: Episode V - The Empire Strikes Back" .
   _:sw2 <release_date> "1980-05-21" .
   _:sw2 <revenue> "534000000" .
   _:sw2 <running_time> "124" .
   _:sw2 <starring> _:luke .
   _:sw2 <starring> _:leia .
   _:sw2 <starring> _:han .
   _:sw2 <director> _:irvin .

   _:sw3 <name> "Star Wars: Episode VI - Return of the Jedi" .
   _:sw3 <release_date> "1983-05-25" .
   _:sw3 <revenue> "572000000" .
   _:sw3 <running_time> "131" .
   _:sw3 <starring> _:luke .
   _:sw3 <starring> _:leia .
   _:sw3 <starring> _:han .
   _:sw3 <director> _:richard .

   _:st1 <name> "Star Trek: The Motion Picture" .
   _:st1 <release_date> "1979-12-07" .
   _:st1 <revenue> "139000000" .
   _:st1 <running_time> "132" .
  }
}
' | python -m json.tool | less
```

Running this next mutation adds a schema and indexes some of the data so queries can use term matching, filtering and sorting.

```sh
curl localhost:8080/query -XPOST -d $'
mutation {
  schema {
    name: string @index .
    release_date: datetime @index .
    revenue: float .
    running_time: int .
  }
}
' | python -m json.tool | less
```

Run this query to get "Star Wars" movies released after "1980".  Try it in the user interface to see the result as a graph.


```sh
curl localhost:8080/query -XPOST -d $'
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

Output

```json
{
    "me": [
        {
            "director": [
                {
                    "name": "Irvin Kernshner"
                }
            ],
            "name": "Star Wars: Episode V - The Empire Strikes Back",
            "release_date": "1980-05-21",
            "revenue": 534000000.0,
            "running_time": 124,
            "starring": [
                {
                    "name": "Han Solo"
                },
                {
                    "name": "Princess Leia"
                },
                {
                    "name": "Luke Skywalker"
                }
            ]
        },
        {
            "director": [
                {
                    "name": "Richard Marquand"
                }
            ],
            "name": "Star Wars: Episode VI - Return of the Jedi",
            "release_date": "1983-05-25",
            "revenue": 572000000.0,
            "running_time": 131,
            "starring": [
                {
                    "name": "Han Solo"
                },
                {
                    "name": "Princess Leia"
                },
                {
                    "name": "Luke Skywalker"
                }
            ]
        }
    ]
}
```




## (Optional) Step 4: Load a bigger dataset

Step 3 showed how to add data with a small mutation.  Bigger datasets can be loaded with dgraphloader.

### Download dataset
Download the goldendata.rdf.gz dataset from [here](https://github.com/dgraph-io/benchmarks/blob/master/data/goldendata.rdf.gz) ([download](https://github.com/dgraph-io/benchmarks/raw/master/data/goldendata.rdf.gz)). Put it directory`~/dgraph`, creating the directory if necessary using `mkdir ~/dgraph`.

```sh
mkdir -p ~/dgraph
cd ~/dgraph
wget "https://github.com/dgraph-io/benchmarks/blob/master/data/goldendata.rdf.gz?raw=true" -O goldendata.rdf.gz -q
```

### Update schema

The schema needs updating to index new predicates in the dataset.  The new dataset also contains a `name` predicate, but it is already indexed from the previous step.

```sh
curl localhost:8080/query -XPOST -d '
mutation {
  schema {
    initial_release_date: datetime @index .
  }
}
'| python -m json.tool | less
```

### Load data with dgraphloader

Load the downloaded dataset by running the following in a terminal.

```sh
cd ~/dgraph # The directory where you downloaded the rdf.gz file.
dgraphloader -r goldendata.rdf.gz
```

### Load data with Docker

If Dgraph was started in Docker, then load the dataset with the following.

```sh
docker exec -it dgraph dgraphloader -r goldendata.rdf.gz
```

### Result

Output

```sh
Processing goldendata.rdf.gz
Number of mutations run   : 1121
Number of RDFs processed  : 1120879
Time spent                : MMmSS.FFFFFFFFs
RDFs processed per second : XXXXX
```

Your counts should be the same, but your statistics will vary.

## (Optional) Step 5: Query Dataset

{{% notice "note" %}} By default, so anyone can run them, these queries run at http://play.dgraph.io, but, if you have followed the above instructions, then the queries can be run and visualized locally by copying to [`http://localhost:8080`](http://localhost:8080).{{% /notice %}}

### Movies by Steven Spielberg

This query finds director "Steven Spielberg" and the movies directed by him.  The movies are sorted by release date in descending order.  A visualization of the graph won't show the order, but the JSON result shows it.

{{< runnable >}}
{
  director(func:allofterms(name, "steven spielberg")) @cascade {
    name@en
    director.film (orderdesc: initial_release_date) {
      name@en
      initial_release_date
    }
  }
}
{{< /runnable >}}


### Released after August 1984

This query filters out some of the results from the previous query.  It still searches for movies by Steven Spielberg, but only those released after August 1984 and ordered by ascending date.

We'll sort in increasing order this time by using `orderasc`, instead of `orderdesc`.

{{< runnable >}}
{
  director(func:allofterms(name, "steven spielberg")) @cascade {
    name@en
    director.film (orderasc: initial_release_date) @filter(ge(initial_release_date, "1984-08")) {
      name@en
      initial_release_date
    }
  }
}
{{< /runnable >}}

### Released in the 1990s

Using `AND` two filters can be joined.

{{< runnable >}}
{
  director(func:allofterms(name, "steven spielberg")) {
    name@en
    director.film (orderasc: initial_release_date) @filter(ge(initial_release_date, "1990") AND le(initial_release_date, "2000")) {
      name@en
      initial_release_date
    }
  }
}
{{< /runnable >}}


### Released since 2016

For the queries so far, the search has started with the name of a director.  But Dgraph can search in many ways.  This query finds films in the dataset released since 2016 and changes the name `initial_release_date` to `released` in the output.

{{< runnable >}}{
  films(func:ge(initial_release_date, "2016")) {
    name@en
    released: initial_release_date
    directed_by {
      name@en
    }
  }
}
{{< /runnable >}}

These queries should give an idea of some of the things Dgraph is capable of.

Take the [tour](https://tour.dgraph.io) for a guided tour of how to write queries in Dgraph.

A wider range of queries can also be found in the [Query Language]({{< relref "query-language/index.md" >}}) reference.



## Other Datasets

The examples in the [Query Language]({{< relref "query-language/index.md" >}}) reference manual use the following datasets.

* A dataset of movies and actors - 21million.rdf.gz [located here](https://github.com/dgraph-io/benchmarks/blob/master/data/21million.rdf.gz), and
* A tourism dataset for geo-location queries - sf.tourism.gz [located here](https://github.com/dgraph-io/benchmarks/blob/master/data/sf.tourism.gz).

To load this data into a local instance of Dgraph.  First, get the data:
```
cd ~/dgraph
wget "https://github.com/dgraph-io/benchmarks/blob/master/data/21million.rdf.gz?raw=true" -O 21million.rdf.gz -q
wget "https://github.com/dgraph-io/benchmarks/blob/master/data/sf.tourism.gz?raw=true" -O sf.tourism.gz -q
```

Then, using the same process as [schema updating]({{< relref "#update-schema" >}}) and [data loading]({{< relref "#load-data-with-dgraphloader" >}}) (or [with Docker]({{< relref "#load-data-with-docker" >}})) from Step 4 above, mutate the schema and load the data files.  The required schema is as follows.

```
mutation {
  schema {
    director.film: uid @reverse .
    genre: uid @reverse .
    initial_release_date: datetime @index .
    rating: uid @reverse .
    country: uid @reverse .
    loc: geo @index .
    name: string @index .
  }
}
```

Depending on the machine used, it can take a few minutes to load the 21 million triples.


## Need Help

* Please use [discuss.dgraph.io](https://discuss.dgraph.io) for questions, feature requests and discussions.
* Please use [Github Issues](https://github.com/dgraph-io/dgraph/issues) if you encounter bugs or have feature requests.
* You can also join our [Slack channel](http://slack.dgraph.io).

## Troubleshooting

### 1. Docker: Error initializing postings store

One of the things to try would be to open bash in the container and try to run Dgraph from within it.

```sh
docker run -it dgraph/dgraph bash
# Now that you are within the container, run Dgraph.
dgraph
```

If Dgraph runs for you that indicates there could be something wrong with mounting volumes.
