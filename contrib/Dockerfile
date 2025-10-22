# This file is used to add the nightly Dgraph binaries and assets to Dgraph base
# image.

# This gets built as part of release.sh. Must be run from /tmp/build, with the linux binaries
# already built and placed there.

FROM ubuntu:24.04
LABEL maintainer="Hypermode Inc. <hello@hypermode.com>"

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
    libjemalloc-dev \
    less \
    sysstat \
    && rm -rf /var/lib/apt/lists/*

ADD linux /usr/local/bin

RUN mkdir /dgraph
WORKDIR /dgraph

ENV GODEBUG=madvdontneed=1
CMD ["dgraph"]
