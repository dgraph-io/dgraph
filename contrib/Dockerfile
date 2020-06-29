# Build the project
FROM golang:1.14.1 as builder

ENV GOPATH=/go
WORKDIR /go/src/dgraph
COPY . .
RUN make install

# Copy the built binary to the runtime container
FROM ubuntu:latest
LABEL maintainer="Dgraph Labs <contact@dgraph.io>"

RUN apt-get update && \
  apt-get install -y --no-install-recommends ca-certificates curl iputils-ping && \
  rm -rf /var/lib/apt/lists/*

COPY --from=builder /go/bin/dgraph /usr/local/bin

EXPOSE 8080
EXPOSE 9080

RUN mkdir /dgraph
WORKDIR /dgraph

CMD ["dgraph"] # Shows the dgraph version and commands available.
