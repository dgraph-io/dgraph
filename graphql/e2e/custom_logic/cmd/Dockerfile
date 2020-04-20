FROM golang:1.12.7

ADD . cmd 

WORKDIR cmd 

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o main main.go

CMD ./main
