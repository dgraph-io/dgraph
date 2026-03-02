FROM dgraph/dgraph:local
LABEL maintainer="Istari Digital, Inc. <dgraph-admin@istaridigital.com>"

RUN apt-get update && apt-get install -y --no-install-recommends \
    nfs-client
CMD ["dgraph"] # Shows the dgraph version and commands available.
