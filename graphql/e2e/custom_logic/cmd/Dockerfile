
FROM golang:1.19-alpine3.17

COPY . .

RUN apk update && apk add git && apk add nodejs && apk add npm

# As of Go 1.16 modules are on by default
RUN go env -w GO111MODULE=auto

RUN go get gopkg.in/yaml.v2

RUN go get github.com/graph-gophers/graphql-go

RUN npm install

RUN go build -o main main.go

WORKDIR .

CMD ./main