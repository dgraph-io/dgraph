# This file is used to add the nightly Dgraph binaries and assets to Dgraph base
# image.

# This gets built as part of release.sh. Must be run from /tmp/build, with the linux binaries
# already built and placed there.

FROM golang:1.14 AS build

WORKDIR /go/src/app
COPY go.sum go.mod ./
RUN go mod download -json
COPY . ./

# Putting these args after the go mod so that the go mod download stays in cache
ARG BUILD
ARG BUILD_DATE
ARG BUILD_BRANCH
ARG BUILD_VERSION

RUN BUILD="${BUILD}" BUILD_DATE="${BUILD_DATE}" BUILD_BRANCH="${BUILD_BRANCH}" BUILD_VERSION="${BUILD_VERSION}" make

# ------------------------------------------------------------------------------

FROM ubuntu:latest
MAINTAINER Dgraph Labs <contact@dgraph.io>

RUN apt-get update && \
  apt-get install -y --no-install-recommends ca-certificates curl iputils-ping && \
  rm -rf /var/lib/apt/lists/*

COPY --from=build /go/src/app/dgraph/dgraph /usr/local/bin/dgraph

EXPOSE 8080
EXPOSE 9080

RUN mkdir /dgraph
WORKDIR /dgraph

CMD ["dgraph"] # Shows the dgraph version and commands available.
