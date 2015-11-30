# Dockerfile for DGraph

FROM golang:1.4.3
MAINTAINER Manish Jain <manishrjain@gmail.com>

# Get the necessary packages.
RUN apt-get update && apt-get install -y --no-install-recommends \
	git \
	libbz2-dev \
	libgflags-dev \
	libsnappy-dev \
	zlib1g-dev \
	&& rm -rf /var/lib/apt/lists/*

# Install and set up RocksDB.
RUN mkdir /installs && cd /installs && \
	git clone --branch v4.1 https://github.com/facebook/rocksdb.git
RUN cd /installs/rocksdb && make shared_lib && make install
ENV LD_LIBRARY_PATH "/usr/local/lib"

# Install DGraph and update dependencies to right versions.
RUN go get -v github.com/robfig/glock && \
	go get -v github.com/dgraph-io/dgraph/... && \
	glock sync github.com/dgraph-io/dgraph && echo "v0.1.2"

# Run some tests, don't build an image if we're failing tests.
RUN go test github.com/dgraph-io/dgraph/...

# Create the dgraph and data directory. These directories should be mapped
# to host machine for persistence.
RUN mkdir /dgraph && mkdir /data
