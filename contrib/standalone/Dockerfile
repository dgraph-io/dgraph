ARG DGRAPH_VERSION=latest
FROM dgraph/dgraph:${DGRAPH_VERSION}
LABEL MAINTAINER="Hypermode Inc. <hello@hypermode.com>"

# Ratel port
# EXPOSE 8000 ## Ratel is not supported in the standalone image
# REST API port
EXPOSE 8080
# gRPC API port
EXPOSE 9080

ADD run.sh /run.sh
RUN chmod +x /run.sh
CMD ["/run.sh"]
