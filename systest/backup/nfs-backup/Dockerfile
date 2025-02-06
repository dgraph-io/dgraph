FROM dgraph/dgraph:local
LABEL maintainer="Hypermode Inc. <hello@hypermode.com>"

RUN apt-get update && apt-get install -y --no-install-recommends \
    nfs-client
CMD ["dgraph"] # Shows the dgraph version and commands available.
