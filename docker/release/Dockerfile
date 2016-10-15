# Dockerfile for Dgraph

# For your convenience:
#   docker build -t dgraph/release .
#   docker run -it dgraph/release bash

# You might see warnings such as the following, but dgraph will run fine.
# Your stdout won't be flooded with these warnings.
#   dgraph: /usr/lib/libstdc++.so.6: no version information available (required by dgraph)

# We need libc (not the lightweight version). Otherwise, dgraph cannot run.
FROM frolvlad/alpine-glibc

RUN apk add --no-cache bash wget curl g++

# Get embedded binaries from Dgraph.
RUN curl https://get.dgraph.io | bash

# Create the dgraph and data directory. These directories should be mapped
# to host machine for persistence.
RUN mkdir /dgraph && mkdir /data

# Some sample sequence of commands:
#   cd /data
#   wget https://github.com/dgraph-io/benchmarks/raw/master/data/rdf-films.gz
#   wget https://github.com/dgraph-io/benchmarks/raw/master/data/names.gz
#   cd /dgraph
#   time dgraphassigner -rdfgzips /data/rdf-films.gz -instanceIdx 0 -numInstances 1 -uids u
#   time dgraphloader   -rdfgzips /data/rdf-films.gz -instanceIdx 0 -numInstances 1 -uids u -postings p
