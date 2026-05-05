# This file is used to add the Dgraph binaries and assets to Dgraph images.
# Ensure that the Dgraph binaries are built and placed in the linux
# directory before building this image.

FROM ubuntu:24.04
LABEL maintainer="Istari Digital, Inc. <dgraph-admin@istaridigital.com>"

# need to remove the cache of sources lists
# apt-get Error Code 100
# https://www.marnel.net/2015/08/apt-get-error-code-100/
RUN rm -rf /var/lib/apt/lists/*

# only update, don't run upgrade
# use cache busting to avoid old versions
# remove /var/lib/apt/lists/* to reduce image size.
# see: https://docs.docker.com/develop/develop-images/dockerfile_best-practices
#
# Retry the install once on transient apt-get errors. The most common
# is the `File has unexpected size... Mirror sync in progress?` race
# against archive.ubuntu.com mid-publish, which surfaces frequently in
# CI. The rm + sleep between attempts gives the mirror time to settle
# and forces a fresh package-list fetch.
RUN set -eu; \
    install_pkgs() { \
        apt-get update && apt-get install -y --no-install-recommends \
            ca-certificates \
            htop \
            curl \
            iputils-ping \
            jq \
            libjemalloc-dev \
            less \
            sysstat \
            gnupg \
            gnupg2 \
            tar \
            libpam0g; \
    }; \
    install_pkgs || (sleep 15 && rm -rf /var/lib/apt/lists/* && install_pkgs); \
    rm -rf /var/lib/apt/lists/*

ADD linux /usr/local/bin

RUN mkdir /dgraph
WORKDIR /dgraph

ENV GODEBUG=madvdontneed=1
CMD ["dgraph"]
