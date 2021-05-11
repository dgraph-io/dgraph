ARG DGRAPH_VERSION=latest
FROM dgraph/dgraph:${DGRAPH_VERSION}
LABEL MAINTAINER="Dgraph Labs <contact@dgraph.io>"

# REST API port
EXPOSE 8080
# gRPC API port
EXPOSE 9080

ADD run.sh /run.sh
RUN chmod +x /run.sh
CMD ["/run.sh"]
