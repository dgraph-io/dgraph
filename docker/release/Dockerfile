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
RUN apt-get install -y --force-yes software-properties-common python-software-properties && sudo add-apt-repository ppa:ubuntu-toolchain-r/test
RUN apt-get update
RUN apt-get install -y --force-yes g++-4.9

# Get embedded binaries from Dgraph.
RUN curl https://get.dgraph.io | bash

# Create the dgraph directory. This directory should be mapped to the host machine for persistence.
RUN mkdir /dgraph

EXPOSE 8080
EXPOSE 9080
WORKDIR /dgraph

CMD ["dgraph"]
