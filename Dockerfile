# This file builds the dgraph binary and dgraph docker image.

# This gets built as part of our CD pipeline. Must be run through Makefile.

# platform flag required on darwin/arm64
FROM --platform=linux/amd64 ubuntu:20.04 AS build

RUN mkdir /dgraph
WORKDIR /dgraph

COPY Makefile ./Makefile
COPY dgraph ./dgraph

ARG DEBIAN_FRONTEND=noninteractive
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update && apt-get install -qq \
    build-essential \
    curl \
    ca-certificates

RUN cd dgraph && make jemalloc

COPY .go-version ./.go-version

RUN GO_VERSION=$({ [ -f .go-version ] && cat .go-version; }) \
    && curl -O -L "https://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
    && tar -C /usr/local/ -xzf go${GO_VERSION}.linux-amd64.tar.gz \
    && rm -f go${GO_VERSION}.linux-amd64.tar.gz

ENV PATH=$PATH:/usr/local/go/bin:/root/go/bin \
    GOPATH=/root/go

COPY . ./

ARG BUILD
ARG BUILD_CODENAME
ARG BUILD_DATE
ARG BUILD_BRANCH
ARG BUILD_VERSION

RUN BUILD=$BUILD \
    BUILD_CODENAME=$BUILD_CODENAME \
    BUILD_DATE=$BUILD_DATE \
    BUILD_BRANCH=$BUILD_BRANCH \
    BUILD_VERSION=$BUILD_VERSION \
    make dgraph

RUN mkdir bin && mv dgraph/dgraph bin

# -------------------------------------------------------------------------------

# platform flag required on darwin/arm64
FROM --platform=linux/amd64 ubuntu:20.04
LABEL maintainer="Dgraph Labs <contact@dgraph.io>"

# need to remove the cache of sources lists
# apt-get Error Code 100
# https://www.marnel.net/2015/08/apt-get-error-code-100/
RUN rm -rf /var/lib/apt/lists/*

# only update, don't run upgrade
# use cache busting to avoid old versions
# remove /var/lib/apt/lists/* to reduce image size. 
# see: https://docs.docker.com/develop/develop-images/dockerfile_best-practices 
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    htop \
    curl \
    htop \
    iputils-ping \
    jq \
    less \
    sysstat \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /dgraph/bin /usr/local/bin

RUN mkdir /dgraph
WORKDIR /dgraph

ENV GODEBUG=madvdontneed=1
CMD ["dgraph"] # Shows the dgraph version and commands available.

