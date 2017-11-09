# This file is used to add the nightly Dgraph binaries and assets to Dgraph base
# image.

# Command to build - docker build -t dgraph/dgraph:nightly .

FROM ubuntu:16.04
MAINTAINER Dgraph Labs <contact@dgraph.io>

RUN mkdir /dgraph \
    && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

ADD dgraph-build /usr/local/bin

RUN mkdir -p /usr/local/share/dgraph/assets
ADD assets.tar.gz /usr/local/share/dgraph/assets

EXPOSE 8080
EXPOSE 9090
WORKDIR /dgraph

CMD ["dgraph"] # Shows the dgraph version and commands available.
