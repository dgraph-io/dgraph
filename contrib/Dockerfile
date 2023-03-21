# This file is used to add the Dgraph binaries to Dgraph base image.

# This gets built as part of our CD pipeline. Must be run through Makefile.

FROM ubuntu:20.04 AS build

RUN mkdir /dgraph
WORKDIR /dgraph

COPY . ./

RUN make dgraph
RUN mkdir bin
RUN mv dgraph/dgraph bin

# ------------------------------------------------------------------------------
#FROM ubuntu:20.04
#EXPOSE 8071

#ARG PROXY_VERSION=924d15f
#ENV PROXY_TAG=$PROXY_VERSION

#COPY --from=build /go/src/app/config ./config
#COPY --from=build /go/src/app/region/config ./region/config
#COPY --from=build /go/src/app/schema.graphql schema.graphql

#COPY --from=build /go/src/app/cerebro /usr/local/bin/cerebro
#CMD ["cerebro"]

# -------------------------------------------------------------------------------

FROM ubuntu:20.04
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

ADD linux /usr/local/bin
COPY --from=build /dgraph/bin ./config

RUN mkdir /dgraph
WORKDIR /dgraph

ENV GODEBUG=madvdontneed=1
CMD ["dgraph"] # Shows the dgraph version and commands available.
