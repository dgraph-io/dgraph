#!/usr/bin/env bash
CWD=$(pwd)
DGRAPH_ROOT=$GOPATH/src/github.com/dgraph-io/dgraph
cd $DGRAPH_ROOT && make install &&\
cd $CWD && docker container ps -a|grep -v CONTAINER|awk '{print $1}'|xargs docker container rm -f && \
docker-compose up --force-recreate
