# dgraph
Distributed Graph Serving System

# Installation
DGraph depends on [RocksDB](https://github.com/facebook/rocksdb).
So, install that first.

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

This would install RocksDB library in /usr/local/lib. Make sure that your `LD_LIBRARY_PATH` is correctly pointing to it.

```
# In ~/.bashrc
export LD_LIBRARY_PATH="/usr/local/lib"
```

Now get [dgraph](https://github.com/dgraph-io/dgraph) code:
```
go get -v github.com/dgraph-io/dgraph/...
```

# Usage

## Data Loading
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

### Loading performance
Loader is memory bound. Every mutation loads a posting list in memory, where mutations
are applied in layers above posting lists.
While loader doesn't write to disk every time a mutation happens, it does periodically
merge all the mutations to posting lists, and writes them to rocksdb which persists them.
How often this merging happens can be fine tuned by specifying `max_ram_mb`.
Every time loader determines it exceeds this threshold, it would *stop the world*, and start the merge process.
The more memory is available for loader to work with, the less merging needs to be done, the faster the loading.

Thus, loader performance is highly dependent on merging performance, which depends on how fast the underlying persistent storage is.
So, *RAMFS/TMPFS > SSD > Hard disk*, when it comes to loading performance.

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
                        film.performance.actor {
                                film.director.film {
                                        type.object.name.en
                                }
                                type.object.name.en
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

The support for GraphQL is very limited right now. In particular, mutations, fragments etc. via GraphQL aren't supported. You can conveniently browse [Freebase film schema here](http://www.freebase.com/film/film?schema=&lang=en). There're also some pointers in dgraph-io/benchmarks/data/README.md.
