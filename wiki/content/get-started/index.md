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

You can check that Dgraph binary installed correctly by running `dgraph` and
looking at its output, which includes the version number.

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
{{% notice "note" %}} This is a set up involving just one machine. For multi-server setup, go to [Deploy]({{< relref "deploy/index.md" >}}). {{% /notice %}}

### From Installed Binary

**Run Dgraph zero**

Run `dgraph zero` to start Dgraph zero. This process controls Dgraph cluster,
maintaining membership information, shard assignment and shard movement, etc.

```sh
dgraph zero
```

**Run Dgraph data server**

Run `dgraph server` with `--memory_mb` flag to start Dgraph server.

```sh
dgraph server --memory_mb 2048
```

{{% notice "tip" %}}You need to set the estimated memory dgraph can take through `memory_mb` flag. This is just a hint to the dgraph and actual usage would be higher than this. It's recommended to set memory_mb to half the available RAM.{{% /notice %}}

#### Windows
To run dgraph with the UI on Windows, you also have to supply the path to the assets using the (`--ui` option).
```sh
./dgraph.exe --memory_mb 2048 -ui path-to-assets-folder
```

### Docker on Linux

```sh
# Directory to store data in. This would be passed to `-v` flag.
mkdir -p /tmp/data

# Run Dgraph Zero
docker run -itP -v /tmp/data:/dgraph --name diggy dgraph/dgraph dgraph zero

# Run Dgraph Server
docker exec -it diggy dgraph server --memory_mb 2048
```

The dgraph server listens on ports 8080 and 9080  with log output to the terminal.

### Docker on Non Linux Distributions.
File access in mounted filesystems is slower when using docker. Try running the command `time dd if=/dev/zero of=test.dat bs=1024 count=100000` on mounted volume and you will notice that it's horribly slow when using mounted volumes. We recommend users to use docker data volumes. The only downside of using data volumes is that you can't access the files from the host, you have to launch a container for accessing it.

{{% notice "tip" %}}If you are using docker on non-linux distribution, please use docker data volumes.{{% /notice %}}

Create a docker data container named datacontainer with dgraph/dgraph image.
```sh
docker create -v /dgraph --name data dgraph/dgraph
```

Now if we run dgraph container with `--volumes-from` flag and run dgraph with the following command, then anything we write to /dgraph in dgraph container will get written to /dgraph volume of datacontainer.
```sh
docker run -itP --volumes-from data --name diggy dgraph/dgraph dgraph zero
docker exec -it diggy dgraph server --memory_mb 2048
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
  name: string @index(term) .
  release_date: datetime @index(year) .
  revenue: float .
  running_time: int .
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
- Take the [Tour](https://tour.dgraph.io) for a guided tour of how to write queries in Dgraph.
- A wider range of queries can also be found in the [Query Language]({{< relref "query-language/index.md" >}}) reference.
- See [Deploy]({{< relref "deploy/index.md" >}}) if you wish to run Dgraph
  in a cluster.

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
