ARG GO_VERSION=1.12
FROM golang:1.12-alpine3.9 as builder

RUN apk --no-cache add git linux-headers ca-certificates
COPY . /go/src/github.com/ChainSafe/gossamer
COPY config.toml /go/src/github.com/ChainSafe/gossamer
WORKDIR /go/src/github.com/ChainSafe/gossamer/cmd
ENV GO111MODULE=on
COPY go.mod .
COPY go.sum .

# Get dependancies - will also be cached if we won't change mod/sum
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o gossamer .

FROM scratch

WORKDIR /
COPY --from=builder ./go/src/github.com/ChainSafe/gossamer/cmd/gossamer/gossamer .
COPY --from=builder ./go/src/github.com/ChainSafe/gossamer/config.toml .
CMD ["./gossamer"]
EXPOSE 7001