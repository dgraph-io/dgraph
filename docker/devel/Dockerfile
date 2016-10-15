# Dockerfile for Dgraph

# For your convenience:
#   docker build -t dgraph/devel .
#   docker run -it dgraph/devel bash

FROM golang:1.7
MAINTAINER Dgraph Labs <contact@dgraph.io>

# Get the necessary packages.
RUN apt-get update && apt-get install -y --no-install-recommends \
	wget \
	git \
	libbz2-dev \
	libgflags-dev \
	libsnappy-dev \
	zlib1g-dev \
	&& rm -rf /var/lib/apt/lists/*

# Install and set up RocksDB.
RUN mkdir /installs && cd /installs && \
	git clone --branch v4.9 https://github.com/facebook/rocksdb.git
RUN cd /installs/rocksdb && \
	make shared_lib && \
	make install

# Install and set up ICU.
RUN cd /installs && \
	wget http://download.icu-project.org/files/icu4c/57.1/icu4c-57_1-src.tgz && \
	tar xvzf icu4c-57_1-src.tgz
RUN cd /installs/icu/source && \
	./configure --disable-renaming && \
	make && \
	make install

ENV CGO_CPPFLAGS "-I$/usr/local/include"
ENV CGO_LDFLAGS "-L/usr/local/lib"
ENV LD_LIBRARY_PATH "/usr/local/lib"

# Install Dgraph and update dependencies to right versions.
# NOTE: Add -d to avoid building everything, which would include cockroachDB
# stuff which we only need for embedding, and can take a long time and
# potentially crash the build.
RUN go get -v -d github.com/dgraph-io/dgraph/... && \
	go build -v github.com/dgraph-io/dgraph/cmd/... && \
	go install -v github.com/dgraph-io/dgraph/cmd/...
	
# TODO: Uncomment and enable tests after they pass.
# RUN go test github.com/dgraph-io/dgraph/...

# Mainly test out RocksDB and ICU.
# RUN go test github.com/dgraph-io/dgraph/tok/...

# Create the dgraph and data directory. These directories should be mapped
# to host machine for persistence.
RUN mkdir /dgraph && mkdir /data
