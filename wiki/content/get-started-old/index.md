+++
title = "Get Started (Old)"
+++

{{% notice "note" %}}This is an older version of the Getting Started page. You can see the latest page [here](../get-started). {{% /notice %}}

## Dgraph

Dgraph cluster consists of different nodes (Zero, Alpha & Ratel) and each node serves a different purpose.

**Dgraph Zero** controls the Dgraph cluster, assigns servers to a group and re-balances data between server groups.

**Dgraph Alpha** hosts predicates and indexes.

**Dgraph Ratel** serves the UI to run queries, mutations & altering schema.

You need at least one Dgraph Zero and one Dgraph Alpha to get started.

**Here's a 3 step tutorial to get you up and running.**

This is a quick-start guide to running Dgraph. For an interactive walk through, take the [tour](https://tour.dgraph.io).

You can see the accompanying [video here](https://www.youtube.com/watch?v=QIIdSp2zLcs).

## Step 1: Install Dgraph

Dgraph can be installed from the install scripts, or run via Docker.

### From Docker Image

Pull the Dgraph Docker images [from here](https://hub.docker.com/r/dgraph/dgraph/). From a terminal:

```sh
docker pull dgraph/dgraph
```

### From Install Scripts (Linux/Mac)

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

You can check that Dgraph binary installed correctly by running `dgraph` and
looking at its output, which includes the version number.

### Installing on Windows

{{% notice "note" %}}Binaries for Windows are available from `v0.8.3`.{{% /notice %}}

If you wish to install the binaries on Windows, you can get them from the [Github releases](https://github.com/dgraph-io/dgraph/releases), extract and install them manually. The file `dgraph-windows-amd64-v1.x.y.tar.gz` contains the dgraph binary.

## Step 2: Run Dgraph
{{% notice "note" %}} This is a set up involving just one machine. For multi-server setup, go to [Deploy](/deploy). {{% /notice %}}

### Docker Compose

The easiest way to get Dgraph up and running is using Docker Compose. Follow the instructions
[here](https://docs.docker.com/compose/install/) to install Docker Compose if you don't have it
already.

```
version: "3.2"
services:
  zero:
    image: dgraph/dgraph:latest
    volumes:
      - type: volume
        source: dgraph
        target: /dgraph
        volume:
          nocopy: true
    ports:
      - 5080:5080
      - 6080:6080
    restart: on-failure
    command: dgraph zero --my=zero:5080
  server:
    image: dgraph/dgraph:latest
    volumes:
      - type: volume
        source: dgraph
        target: /dgraph
        volume:
          nocopy: true
    ports:
      - 8080:8080
      - 9080:9080
    restart: on-failure
    command: dgraph alpha --my=server:7080 --lru_mb=2048 --zero=zero:5080
  ratel:
    image: dgraph/dgraph:latest
    volumes:
      - type: volume
        source: dgraph
        target: /dgraph
        volume:
          nocopy: true
    ports:
      - 8000:8000
    command: dgraph-ratel

volumes:
  dgraph:
```

Save the contents of the snippet above in a file called `docker-compose.yml`, then run the following
command from the folder containing the file.
```
docker-compose up -d
```

This would start Dgraph Alpha, Zero and Ratel. You can check the logs using `docker-compose logs`

### From Installed Binary

**Run Dgraph zero**

Run `dgraph zero` to start Dgraph zero. This process controls Dgraph cluster,
maintaining membership information, shard assignment and shard movement, etc.

```sh
dgraph zero
```

**Run Dgraph data server**

Run `dgraph alpha` to start Dgraph alpha.

```sh
dgraph alpha --lru_mb 2048 --zero localhost:5080
```

**Run Ratel**

Run 'dgraph-ratel' to start Dgraph UI. This can be used to do mutations and query through UI.

```sh
dgraph-ratel
```

{{% notice "tip" %}}You need to set the estimated memory Dgraph alpha can take through `lru_mb` flag. This is just a hint to the Dgraph alpha and actual usage would be higher than this. It's recommended to set lru_mb to one-third the available RAM.{{% /notice %}}

### Docker on Linux

```sh
# Directory to store data in. This would be passed to `-v` flag.
mkdir -p /tmp/data

# Run Dgraph Zero
docker run -it -p 5080:5080 -p 6080:6080 -p 8080:8080 -p 9080:9080 -p 8000:8000 -v /tmp/data:/dgraph --name diggy dgraph/dgraph dgraph zero

# Run Dgraph Alpha
docker exec -it diggy dgraph alpha --lru_mb 2048 --zero localhost:5080

# Run Dgraph Ratel
docker exec -it diggy dgraph-ratel
```

The dgraph alpha listens on ports 8080 and 9080  with log output to the terminal.

### Docker on Non Linux Distributions.
File access in mounted filesystems is slower when using docker. Try running the command `time dd if=/dev/zero of=test.dat bs=1024 count=100000` on mounted volume and you will notice that it's horribly slow when using mounted volumes. We recommend users to use docker data volumes. The only downside of using data volumes is that you can't access the files from the host, you have to launch a container for accessing it.

{{% notice "tip" %}}If you are using docker on non-linux distribution, please use docker data volumes.{{% /notice %}}

Create a docker data container named *data* with dgraph/dgraph image.
```sh
docker create -v /dgraph --name data dgraph/dgraph
```

Now if we run Dgraph container with `--volumes-from` flag and run Dgraph with the following command, then anything we write to /dgraph in Dgraph container will get written to /dgraph volume of datacontainer.
```sh
docker run -it -p 5080:5080 -p 6080:6080 --volumes-from data --name diggy dgraph/dgraph dgraph zero
docker exec -it diggy dgraph alpha --lru_mb 2048 --zero localhost:5080

# Run Dgraph Ratel
docker exec -it diggy dgraph-ratel
```

{{% notice "tip" %}}
If you are using Dgraph v1.0.2 (or older) then the default ports are 7080, 8080 for zero, so when following instructions for different setup guides override zero port using `--port_offset`.

```sh
dgraph zero --lru_mb=<typically one-third the RAM> --port_offset -2000
```
Ratel's default port is 8081, so override it using -p 8000.

{{% /notice %}}


## Step 3: Run Queries
{{% notice "tip" %}}Once Dgraph is running, you can access Ratel at [`http://localhost:8000`](http://localhost:8000). It allows browser-based queries, mutations and visualizations.

The mutations and queries below can either be run from the command line using `curl -H "Content-Type: application/rdf" localhost:8080/query -XPOST -d $'...'` or by pasting everything between the two `'` into the running user interface on localhost.{{% /notice %}}

### Dataset
The dataset is a movie graph, where and the graph nodes are entities of the type directors, actors, genres, or movies.

### Storing data in the graph
Changing the data stored in Dgraph is a mutation.  The following mutation stores information about the first three releases of the the ''Star Wars'' series and one of the ''Star Trek'' movies.  Running this mutation, either through the UI or on the command line, will store the data in Dgraph.


```sh
curl -H "Content-Type: application/rdf" localhost:8080/mutate?commitNow=true -XPOST -d $'
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
To run an RDF mutation via curl, you can use the curl option `--data-binary @/path/to/mutation.txt` instead of `-d $''`. The `--data-binary` option skips curl's default URL-encoding.
{{% /notice %}}

### Adding indexes
Alter the schema to add indexes on some of the data so queries can use term matching, filtering and sorting.

```sh
curl localhost:8080/alter -XPOST -d $'
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

### Get all movies
Run this query to get all the movies. The query works below all the movies have a starring edge

```sh
curl -H "Content-Type: application/graphql+-" localhost:8080/query -XPOST -d $'
{
 me(func: has(starring)) {
   name
  }
}
' | python -m json.tool | less
```

### Get all movies released after "1980"
Run this query to get "Star Wars" movies released after "1980".  Try it in the user interface to see the result as a graph.


```sh
curl -H "Content-Type: application/graphql+-" localhost:8080/query -XPOST -d $'
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

That's it! In these three steps, we set up Dgraph, added some data, set a schema
and queried that data back.

## Where to go from here

- Go to [Clients]({{< relref "clients/index.md" >}}) to see how to communicate
with Dgraph from your application.
- Take the [Tour](https://tour.dgraph.io) for a guided tour of how to write queries in Dgraph.
- A wider range of queries can also be found in the [Query Language](/query-language) reference.
- See [Deploy](/deploy) if you wish to run Dgraph
  in a cluster.

## Need Help

* Please use [discuss.dgraph.io](https://discuss.dgraph.io) for questions, feature requests and discussions.
* Please use [Github Issues](https://github.com/dgraph-io/dgraph/issues) if you encounter bugs or have feature requests.
* You can also join our [Slack channel](http://slack.dgraph.io).

## Troubleshooting

### 1. Docker: Error response from daemon; Conflict. Container name already exists.

Remove the diggy container and try the docker run command again.
```
docker rm diggy
```
