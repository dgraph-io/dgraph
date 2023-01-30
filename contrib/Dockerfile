# This file is used to add the nightly Dgraph binaries and assets to Dgraph base
# image.

# This gets built as part of release.sh. Must be run from /tmp/build, with the linux binaries
# already built and placed there.

FROM ubuntu:20.04
LABEL maintainer="Dgraph Labs <contact@dgraph.io>"

# need to remove the cache of sources lists
# apt-get Error Code 100
# https://www.marnel.net/2015/08/apt-get-error-code-100/
RUN rm -rf /var/lib/apt/lists/*

# only update, don't run upgrade
RUN apt-get update
# pin package versions always & bring in CVE fixes as needed
RUN apt-get install -y --no-install-recommends ca-certificates
RUN apt-get install -y --no-install-recommends curl
RUN apt-get install -y --no-install-recommends htop
RUN apt-get install -y --no-install-recommends iputils-ping
RUN apt-get install -y --no-install-recommends jq
RUN apt-get install -y --no-install-recommends less
RUN apt-get install -y --no-install-recommends sysstat

ADD linux /usr/local/bin

EXPOSE 8080
EXPOSE 9080

RUN mkdir /dgraph
WORKDIR /dgraph

ENV GODEBUG=madvdontneed=1
CMD ["dgraph"] # Shows the dgraph version and commands available.
