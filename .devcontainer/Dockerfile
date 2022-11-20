FROM golang:1.18.8-alpine3.16
LABEL maintainer="Dgraph Labs <contact@dgraph.io>"

RUN apk add --update --no-cache docker openrc docker-compose \
    libc-dev make gcc protobuf curl bash git \
    && rc-update add docker boot 

RUN go install -v golang.org/x/tools/gopls@latest 2>&1 \
    && go install -v github.com/go-delve/delve/cmd/dlv@latest

CMD /bin/sh
