# This docker file is used to create the base image used to upload the nightly
# builds to Github. It contains all the dependencies for the embedded Dgraph binary
# to work.

# Command to build - docker build -t dgraph/dgraph:base .
FROM ubuntu:16.04
MAINTAINER Dgraph Labs <contact@dgraph.io>

RUN mkdir /dgraph \
    && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /dgraph

