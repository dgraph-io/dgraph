# This file is used to add the nightly Dgraph binaries and assets to Dgraph base
# image.

# This gets built as part of release.sh. Must be run from /tmp/build, with the linux binaries
# already built and placed there.

FROM ubuntu:20.04
LABEL maintainer="Dgraph Labs <contact@dgraph.io>"

# only update, don't run upgrade
RUN apt-get update
# pin package versions always & bring in CVE fixes as needed
RUN apt-get install -y --no-install-recommends ca-certificates
RUN apt-get install -y --no-install-recommends curl="7.68.0-1ubuntu2.14"
RUN apt-get install -y --no-install-recommends htop="2.2.0-2build1"
RUN apt-get install -y --no-install-recommends iputils-ping="3:20190709-3"
RUN apt-get install -y --no-install-recommends jq="1.6-1ubuntu0.20.04.1"
RUN apt-get install -y --no-install-recommends less="551-1ubuntu0.1"
RUN apt-get install -y --no-install-recommends sysstat

# rm -rf /var/lib/apt/lists/* # TODO: clean this up only if necessary

ADD linux /usr/local/bin

EXPOSE 8080
EXPOSE 9080

RUN mkdir /dgraph
WORKDIR /dgraph

ENV GODEBUG=madvdontneed=1
CMD ["dgraph"] # Shows the dgraph version and commands available.
