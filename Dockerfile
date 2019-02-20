FROM golang:1.11.5-alpine3.8 as build

RUN apk --no-cache add git linux-headers ca-certificates
COPY . /go/src/github.com/ChainSafeSystems/go-pre
WORKDIR /go/src/github.com/ChainSafeSystems/go-pre
ENV GO111MODULE=off
RUN go install -v ./...
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o go-pre .


FROM scratch

WORKDIR /
COPY --from=builder ./go/src/github.com/ChainSafeSystems/go-pre/go-pre .
CMD ["./go-pre"]