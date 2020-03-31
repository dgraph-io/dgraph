FROM golang:1.12.7-alpine3.10

COPY main.go cmd/main.go

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o main cmd/main.go

CMD ./main
