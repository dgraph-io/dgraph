# Dockerfile for Dgraph

# For your convenience:
#   docker build -t dgraph/dgraph .
#
# To run dgraph:
#   docker run -it dgraph/dgraph
# To go into bash:
#   docker run -it dgraph/dgraph bash

FROM ubuntu:14.04
MAINTAINER Dgraph Labs <contact@dgraph.io>

RUN apt-get update

RUN apt-get -y --force-yes install wget curl tar

# Get embedded binaries from Dgraph.
RUN curl https://get.dgraph.io | bash

# Create the dgraph and data directory. These directories should be mapped
# to host machine for persistence.
RUN mkdir /dgraph && mkdir /data

# Sequence of commands to bulkload some sample data:
#   cd /data
#   wget https://github.com/dgraph-io/benchmarks/raw/master/data/rdf-films.gz
#   wget https://github.com/dgraph-io/benchmarks/raw/master/data/names.gz
#   cd /dgraph
#   time dgraphassigner -rdfgzips /data/rdf-films.gz -instanceIdx 0 -numInstances 1 -uids u
#   time dgraphloader   -rdfgzips /data/rdf-films.gz -instanceIdx 0 -numInstances 1 -uids u -postings p

EXPOSE 8080
WORKDIR /dgraph
CMD ["dgraph"]
