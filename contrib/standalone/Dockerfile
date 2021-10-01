ARG DGRAPH_VERSION=latest
FROM dgraph/dgraph:${DGRAPH_VERSION}
LABEL MAINTAINER="Dgraph Labs <contact@dgraph.io>"

# alpha REST API port
EXPOSE 8080
# alpha gRPC API port
EXPOSE 9080
# zero admin REST API port
EXPOSE 6080

ADD run.sh /run.sh
RUN chmod +x /run.sh
CMD ["/run.sh"]
