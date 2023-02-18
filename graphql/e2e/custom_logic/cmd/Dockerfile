
FROM golang:1.19-alpine3.17

COPY . .

RUN apk update && apk add git && apk add nodejs && apk add npm

RUN go install gopkg.in/yaml.v2@latest

RUN go install github.com/graph-gophers/graphql-go@latest

RUN npm install

RUN go build -o main main.go

WORKDIR .

CMD ./main