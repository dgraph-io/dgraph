# This file is used to add the nightly Dgraph binaries and assets to Dgraph base
# image.

FROM dgraph/dgraph:base
MAINTAINER Dgraph Labs <contact@dgraph.io>

ADD dgraph /usr/local/bin

RUN mkdir -p /usr/local/share/dgraph/assets
ADD assets.tar.gz /usr/local/share/dgraph/assets

CMD ["dgraph"]
