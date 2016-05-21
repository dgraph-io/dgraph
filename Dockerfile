# Dockerfile for DGraph
FROM golang:1.6.2-alpine

MAINTAINER Manish Jain <manishrjain@gmail.com>

ENV DGRAPH github.com/dgraph-io/dgraph

COPY . $GOPATH/src/$DGRAPH
RUN mkdir -p $GOPATH/src/$DGRAPH && \
    mkdir /dgraph && mkdir /data && \
    sed -i -e 's/v3\.3/edge/g' /etc/apk/repositories; \
    echo 'http://dl-4.alpinelinux.org/alpine/edge/community' >> /etc/apk/repositories && \
    echo 'http://dl-4.alpinelinux.org/alpine/edge/main' >> /etc/apk/repositories && \
    echo 'http://dl-4.alpinelinux.org/alpine/edge/testing' >> /etc/apk/repositories && \
    apk update; \
    apk add rocksdb rocksdb-dev build-base

# Install DGraph, test, and clean up.
RUN go build -v $DGRAPH/... && \
    go test $DGRAPH/... && echo "v0.2.3" && \
    apk del rocksdb-dev build-base go && rm -rf /var/cache/apk/*
