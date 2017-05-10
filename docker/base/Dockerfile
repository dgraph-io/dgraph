# This docker file is used to create the base image used to upload the nightly
# builds to Github. It contains all the dependencies for the embedded Dgraph binary
# to work.

# Command to build - docker build -t dgraph/dgraph:base .
FROM ubuntu:16.04
MAINTAINER Dgraph Labs <contact@dgraph.io>

RUN apt-get update

RUN apt-get -y --force-yes install wget curl tar

RUN mkdir /dgraph && mkdir /data

WORKDIR /dgraph
