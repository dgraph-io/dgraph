
FROM golang:1.14.2-alpine3.11

COPY . .

RUN apk update && apk add git && apk add nodejs && apk add npm

RUN go get gopkg.in/yaml.v2

RUN go get github.com/graph-gophers/graphql-go/...

RUN npm install

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o main main.go

WORKDIR .

CMD ./main