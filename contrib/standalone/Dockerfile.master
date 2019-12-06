FROM dgraph/dgraph:master
LABEL MAINTAINER="Dgraph Labs <contact@dgraph.io>"

# Ratel port
EXPOSE 8000
# REST API port
EXPOSE 8080
# gRPC API port
EXPOSE 9080

ADD run.sh .
RUN chmod +x run.sh
CMD ["./run.sh"]
