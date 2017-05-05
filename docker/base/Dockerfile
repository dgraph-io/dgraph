# This docker file is used to create the base image used to upload the nightly
# builds to Github. It contains all the dependencies for the embedded Dgraph binary
# to work.

FROM ubuntu:14.04
MAINTAINER Dgraph Labs <contact@dgraph.io>

RUN apt-get update

RUN apt-get -y --force-yes install wget curl tar
RUN apt-get install -y --force-yes software-properties-common python-software-properties && sudo add-apt-repository ppa:ubuntu-toolchain-r/test
RUN apt-get update
RUN apt-get install -y --force-yes g++-4.9

RUN mkdir /dgraph && mkdir /data

WORKDIR /dgraph
