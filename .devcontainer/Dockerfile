FROM golang:1.18-alpine
LABEL maintainer Dgraph.io
RUN apk add libc-dev make gcc protobuf
RUN go install -v golang.org/x/tools/gopls@latest 2>&1
RUN go install -v github.com/go-delve/delve/cmd/dlv@latest

CMD /bin/sh
# docker build -t godebug:1.0 .