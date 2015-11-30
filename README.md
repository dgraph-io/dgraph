# DGraph
**Open Source, Distributed, Low Latency, High Throughput Graph Database.**

![logo](https://img.shields.io/badge/status-alpha-red.svg)
[![logo](https://img.shields.io/badge/Mailing%20List-dgraph-brightgreen.svg)](https://groups.google.com/forum/#!forum/dgraph)

DGraph's goal is to provide [Google](https://www.google.com) level production latency and scale,
with low enough latency to be serving real time user queries, over terabytes of structured data.

View [5 min presentation](http://go-talks.appspot.com/github.com/dgraph-io/dgraph/present/sydney5mins/g.slide#1) at Go meetup, Sydney.

# Current Status

`MVP launch - Dec 2015`
This is a minimum viable product, alpha release of DGraph. **It's not meant for production use.**
See the [Roadmap](https://github.com/dgraph-io/dgraph/issues/1) for list of working and planned features.

Your feedback is welcome. Feel free to [file an issue](https://github.com/dgraph-io/dgraph/issues)
to direct the development of DGraph.

There's an instance of DGraph running at http://dgraph.xyz, that you can query without installing DGraph.
This instance contains 21M facts from Freebase Film Data. See the [query section below](#querying) for a sample query.
`curl dgraph.xyz/query -XPOST -d '{}'`

# Installation

## Via Docker
There's a docker image that you can readily use.
```
$ docker pull dgraph/dgraph:latest
$ docker run -t -i -v /somedir:/dgraph -v $HOME/go/src/github.com/dgraph-io/benchmarks/data:/data -p 8080:8080 dgraph/dgraph:latest
```

Once into the dgraph container, you can now load your data. See [Data Loading](#data-loading) below.
Also, you can skip this step, if you just want to play with DGraph. See [Use Freebase Film data](#use-freebase-film-data).
```
$ loader --postings /dgraph/p --rdfgzips /data/rdf-data.gzip --max_ram_mb 3000
```
Once done, you can start the server
```
$ mkdir /dgraph/m  # Ensure mutations directory exists.
$ server --postings /dgraph/p --mutations /dgraph/m  --max_ram_mb 3000
```

Now you can query the server, like so:
```
$ curl localhost:8080/query -XPOST -d '{root(_xid_: g.11b7nwjrxk) {type.object.name.en}}'
```

## Directly on host machine
Best way to do this is to refer to [Dockerfile](Dockerfile), which has the most complete
instructions on getting the right setup.
All the instructions below are based on a Debian/Ubuntu system.

### Install Go 1.4
Go 1.5 has a regression bug in `cgo`, due to which DGraph is dependent on Go1.4.
So [download and install Go 1.4.3](https://golang.org/dl/).

### Install RocksDB
DGraph depends on [RocksDB](https://github.com/facebook/rocksdb) for storing posting lists.

```
# First install dependencies.
# For Ubuntu, follow the ones below. For others, refer to INSTALL file in rocksdb.
$ sudo apt-get install libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev
$ git clone https://github.com/facebook/rocksdb.git
$ cd rocksdb
$ git checkout v4.1
$ make shared_lib
$ sudo make install
```

This would install RocksDB library in `/usr/local/lib`. Make sure that your `LD_LIBRARY_PATH` is correctly pointing to it.

```
# In ~/.bashrc
export LD_LIBRARY_PATH="/usr/local/lib"
```

### Install DGraph
Now get [DGraph](https://github.com/dgraph-io/dgraph) code. DGraph uses `glock` to fix dependency versions.
```
go get -v github.com/robfig/glock
go get -v github.com/dgraph-io/dgraph/...
glock sync github.com/dgraph-io/dgraph

# Optional
go test github.com/dgraph-io/dgraph/...
```

# Usage

## Data Loading
### Use Freebase Film data
If you just want to play with the system, you can [download this postings directory](https://www.dropbox.com/s/o0lghhd6u7e9eiq/dgraph-p.tar.gz?dl=0),
unzip/untar it and skip right to [Querying](#querying). This directory contains all the Freebase film data in DGraph posting lists format.

### Bulk Loading
Let's load up data first. If you have RDF data, you can use that.
Or, there's [Freebase film rdf data here](https://github.com/dgraph-io/benchmarks).

To use the above mentioned Film RDF data, install [Git LFS first](https://git-lfs.github.com/). I've found the Linux download to be the easiest way to install.
Once installed, clone the repository:
```
$ git clone https://github.com/dgraph-io/benchmarks.git
```

To load the data in bulk, use the data loader binary in dgraph/server/loader.
Loader needs a postings directory, where posting lists are stored.

```
$ cd $GOPATH/src/github.com/dgraph-io/dgraph/server/loader
$ go build . && ./loader --rdfgzips=path_of_benchmarks_dir/data/rdf-films.gz,path_of_benchmarks_dir/data/names.gz --postings DIRPATH/p
```

#### Loading performance
Loader is memory bound. Every mutation loads a posting list in memory, where mutations
are applied in layers above posting lists.
While loader doesn't write to disk every time a mutation happens, it does periodically
merge all the mutations to posting lists, and writes them to rocksdb which persists them.
How often this merging happens can be fine tuned by specifying `max_ram_mb`.
Periodically loader checks it's memory usage and if determines it exceeds this threshold,
it would *stop the world*, and start the merge process.
The more memory is available for loader to work with, the less frequently merging needs to be done, the faster the loading.

In other words, loader performance is highly dependent on merging performance, which depends on how fast the underlying persistent storage is.
So, *Ramfs/Tmpfs > SSD > Hard disk*, when it comes to loading performance.

As a reference point, it takes 220 seconds to load 4.1M RDFs from `names.gz`(from benchmarks repository) on my 6-core Intel Xeon Dell Precision T3500, using 1G TMPFS for postings directory, and with `max_ram_mb=3000` flag set.

## Querying
Once data is loaded, point the dgraph server to the postings and mutations directory.
```
$ cd $GOPATH/src/github.com/dgraph-io/dgraph/server
$ go build .
$ ./server --mutations DIRPATH/m --postings DIRPATH/p
```

This would now run dgraph server at port 8080. If you want to run it at some other port, you can change that with the `--port` flag.

Now you can run GraphQL queries over freebase film data like so:
```
curl localhost:8080/query -XPOST -d '{
	me(_xid_: m.06pj8) {
		type.object.name.en
		film.director.film {
			type.object.name.en
			film.film.starring {
				film.performance.character {
					type.object.name.en
				}
				film.performance.actor {
					type.object.name.en
					film.director.film {
						type.object.name.en
					}
				}
			}
			film.film.initial_release_date
			film.film.country
			film.film.genre {
				type.object.name.en
			}
		}
	}
}' > output.json
```
This query would find all movies directed by Steven Spielberg, their names, initial release dates, countries, genres, and the cast of these movies, i.e. characteres and actors playing those characters; and all the movies directed by these actors, if any.

The support for GraphQL is [very limited right now](https://github.com/dgraph-io/dgraph/issues/1). In particular, mutations, fragments etc. via GraphQL aren't supported.
You can conveniently browse [Freebase film schema here](http://www.freebase.com/film/film?schema=&lang=en).
There're also some schema pointers in [README](https://github.com/dgraph-io/benchmarks/blob/master/data/README.md).

# Contact
- Please direct your questions to [dgraph@googlegroups.com](mailto:dgraph@googlegroups.com).
