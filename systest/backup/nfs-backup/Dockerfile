FROM dgraph/dgraph:local
LABEL maintainer="Dgraph Labs <contact@dgraph.io>"

RUN apt-get update && apt-get install -y --no-install-recommends \
    nfs-client
CMD ["dgraph"] # Shows the dgraph version and commands available.
