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

# Export the ICU DATA variable.
ENV ICU_DATA /usr/local/share/icudt58l.dat

EXPOSE 8080
WORKDIR /dgraph
# Get the golden data schema.
RUN wget "https://github.com/dgraph-io/benchmarks/blob/master/data/goldendata.schema?raw=true" -O goldendata.schema -q

CMD ["dgraph"]
