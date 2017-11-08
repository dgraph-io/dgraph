+++
title = "Get Started"
+++

**New to Dgraph? Here's a 3 step tutorial to get you up and running.**

This is a quick-start guide to running Dgraph. For an interactive walk through, take the [tour](https://tour.dgraph.io).

You can see the accompanying [video here](https://www.youtube.com/watch?v=QIIdSp2zLcs).
## Step 1: Install Dgraph

Dgraph can be installed from the install scripts, or run via Docker.

{{% notice "note" %}}These instructions will install the latest release version.  To instead install our nightly build see [these instructions]({{< relref "deploy/index.md#nightly" >}}).{{% /notice %}}

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

### From Docker Image

Pull the Dgraph Docker images [from here](https://hub.docker.com/r/dgraph/dgraph/). From a terminal:

```sh
docker pull dgraph/dgraph
```

### Installing on Windows

{{% notice "note" %}}Binaries for Windows are available from `v0.8.3`.{{% /notice %}}

If you wish to install the binaries on Windows, you can get them from the [Github releases](https://github.com/dgraph-io/dgraph/releases), extract and install them manually. The file `dgraph-windows-amd64-v0.x.y.tar.gz` contains
all the binaries.

If you wish to run the UI for Dgraph you should also download the `assets.tar.gz` and extract them into a folder called assets.
```sh
mkdir assets && tar -xzvf assets.tar.gz -C assets
```


## Step 2: Run Dgraph
{{% notice "note" %}}You need to set the estimated memory dgraph can take through `memory_mb` flag. This is just a hint to the dgraph and actual usage would be higher than this. It's recommended to set memory_mb to half the size of RAM.{{% /notice %}}

### From Installed Binary

**Run Dgraph zero**

Run `dgraph zero` to start Dgraph zero. This process controls Dgraph cluster,
maintaining membership information, shard assignment and shard movement, etc.

```sh
dgraph zero

# dgraph zero --my "IP:PORT"
```

Run `dgraph zero --help` to see the full list of flags and their default values.
Unless you want high availability, running just one `zero` process for the
entire Dgraph cluster is sufficient.

**Run Dgraph data server**

Run `dgraph server` with `--memory_mb` flag to start Dgraph server.

```sh
dgraph server --memory_mb 2048

# dgraph server --memory_mb 2048 --my "IP:PORT" --zero "IP:PORT"
```

If Dgraph
data server is running on a different machine than Dgraph Zero, then you'd also
want to set `--my` and `--zero` flags, so the two processes can talk to each
other.

If you want to shard your data for horizontal scability, run more Dgraph data
servers, like above. Typically, the number of shards would be equal to the
number of Dgraph data servers divided by value of `--replicas` flag in Zero.

Run `dgraph server --help` to see the full list of available flags and their
default values.

#### High availability setup [optional]

If you want to maintain high availability, you could run multiple Dgraph Zero
processes, and have them talk to each other by providing `--peer` flag.

By default, Dgraph Zero would set each data shard to be served by exactly one
Dgraph server. If you want to have the shards be replicated, you could set the
`--replicas` flag to a value greater than 1.

Note that to form a valid consensus, the number of Zero servers should be odd.
So, that means, having 1, 3 or 5 Zero servers is ideal.

Similarly, to form consensus among Dgraph replicas, you'd want to set the
`--replicas` flag to 1, 3, or 5, which would replicate the data corresponding
number of times.


#### Windows
To run dgraph with the UI on Windows, you also have to supply the path to the assets using the (`--ui` option).
```sh
./dgraph.exe --memory_mb 2048 --zero 127.0.0.1:8888 -ui path-to-assets-folder
```

### Using Docker

The `-v` flag lets Docker mount a directory so that dgraph can persist data to disk and access files for loading data.

#### Map to default ports (8080 and 9080)

Run `dgraphzero`
```sh
mkdir -p ~/dgraph
docker run -it -p 8080:8080 -p 9080:9080 -v ~/dgraph:/dgraph --name dgraph dgraph/dgraph dgraphzero -w zw
```

Run `dgraph`
```sh
docker exec -it dgraph dgraph --bindall=true --memory_mb 2048 -peer 127.0.0.1:8888
```

#### Map to custom port
```sh
mkdir -p ~/dgraph
# Mapping port 8080 from within the container to 18080 of the instance, likewise with the gRPC port 9080.
docker run -it -p 18080:8080 -p 19090:9080 -v ~/dgraph:/dgraph --name dgraph dgraph/dgraph dgraphzero -w zw
docker exec -it dgraph dgraph --bindall=true --memory_mb 2048 -peer 127.0.0.1:8888
```

{{% notice "note" %}}The dgraph server listens on ports 8080 and 9080 (unless mapped to another port above) with log output to the terminal.{{% /notice %}}

{{% notice "note" %}}If you are using docker on non-linux distribution, please use docker data volumes.{{% /notice %}}
### On Non Linux Distributions.
File access in mounted filesystems is slower when using docker. Try running the command `time dd if=/dev/zero of=test.dat bs=1024 count=100000` on mounted volume and you will notice that it's horribly slow when using mounted volumes. We recommend users to use docker data volumes. The only downside of using data volumes is that you can't access the files from the host, you have to launch a container for accessing it.

Create a docker data container named datacontainer with dgraph/dgraph image.
```sh
docker create -v /dgraph --name datacontainer dgraph/dgraph
```

Now if we run dgraph container with `--volumes-from` flag and run dgraph with the following command, then anything we write to /dgraph in dgraph container will get written to /dgraph volume of datacontainer.
```sh
docker run -it -p 18080:8080 -p 19090:9080 --volumes-from datacontainer --name dgraph dgraph/dgraph dgraphzero -w zw
docker exec -it dgraph dgraph --bindall=true --memory_mb 2048 --p /dgraph/p --w /dgraph/w -peer 127.0.0.1:8888
```

## Step 3: Run Queries
{{% notice "tip" %}}Once Dgraph is running, a user interface is available at [`http://localhost:8080`](http://localhost:8080).  It allows browser-based queries, mutations and visualizations.

The mutations and queries below can either be run from the command line using `curl localhost:8080/query -XPOST -d $'...'` or by pasting everything between the two `'` into the running user interface on localhost.{{% /notice %}}


Changing the data or schema stored in Dgraph is a mutation.  The following mutation stores information about the first three releases of the the ''Star Wars'' series and one of the ''Star Trek'' movies.  Running this mutation, either through the UI or on the command line, will store the data in Dgraph.


```sh
curl localhost:8080/mutate -H "X-Dgraph-CommitNow: true" -XPOST -d $'
{
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
curl localhost:8080/alter -XPOST -d $'
{
  "schema": "
    name: string @index(term) .
    release_date: datetime @index(year) .
    revenue: float .
    running_time: int ."
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
- Take the [tour](https://tour.dgraph.io) for a guided tour of how to write queries in Dgraph.
- A wider range of queries can also be found in the [Query Language]({{< relref "query-language/index.md" >}}) reference.

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
dgraph server --memory_mb 2048
```

If Dgraph runs for you that indicates there could be something wrong with mounting volumes.

### 2. Docker: Error response from daemon; Conflict. Container name already exists.

Remove the dgraph container and try the docker run command again.
```
docker rm dgraph
```
