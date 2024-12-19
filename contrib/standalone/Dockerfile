ARG DGRAPH_VERSION=latest
FROM dgraph/dgraph:${DGRAPH_VERSION}
LABEL MAINTAINER="Hypermode <hello@hypermode.com>"

# Zero port
EXPOSE 5080
# REST API port
EXPOSE 8080
# gRPC API port
EXPOSE 9080

ADD load_and_run.sh /load_and_run.sh
RUN chmod +x /load_and_run.sh
CMD ["/load_and_run.sh"]
