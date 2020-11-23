+++
title = "Lambda Server"
weight = 5
[menu.main]
    parent = "lambda"
+++

In this article you'll learn how to setup a Dgraph database with a lambda server.

## Dgraph Lambda

[Dgraph Lambda](https://github.com/dgraph-io/dgraph-lambda) is a serverless platform for running JavaScript on Dgraph and [Slash GraphQL](https://dgraph.io/slash-graphql).

You can [download the latest version](https://github.com/dgraph-io/dgraph-lambda/releases/latest) or review the implementation in our [open-source repository](https://github.com/dgraph-io/dgraph-lambda).

### Running with Docker

To run a Dgraph Lambda server with Docker:

```bash
docker run -it --rm -p 8686:8686 -v /path/to/script.js:/app/script/script.js -e DGRAPH_URL=http://host.docker.internal:8080 dgraph/dgraph-lambda
```

{{% notice "note" %}}
`host.docker.internal` doesn't work on older versions of Docker on Linux. You can use `DGRAPH_URL=http://172.17.0.1:8080` instead.
{{% /notice %}}


### Adding libraries

If you would like to add libraries to Dgraph Lambda, use `webpack --target=webworker` to compile your script.

### Working with TypeScript

You can import `@slash-graph/lambda-types` to get types for `addGraphQLResolver` and `addGraphQLMultiParentResolver`.


## Dgraph Alpha

To set up Dgraph Alpha, you need to define the `--graphql_lambda_url` flag, which is used to set the URL of the lambda server. All the `@lambda` fields will be resolved through the lambda functions implemented on the given lambda server.

For example:

```bash
dgraph alpha --graphql_lambda_url=http://localhost:8686/graphql-worker
```

Then test it out with the following `curl` command:
```bash
curl localhost:8686/graphql-worker -H "Content-Type: application/json" -d '{"resolver":"MyType.customField","parent":[{"customField":"Dgraph Labs"}]}'
```

### Docker settings

If you're using Docker, you need to add the `--graphql_lambda_url` to your Alpha configuration. For example:

```yml
    command: /gobin/dgraph alpha --zero=zero1:5180 -o 100 --expose_trace --trace 1.0
      --profile_mode block --block_rate 10 --logtostderr -v=2
      --whitelist 10.0.0.0/8,172.16.0.0/12,192.168.0.0/16 --my=alpha1:7180
      --graphql_lambda_url=http://lambda:8686/graphql-worker
```

Next, you need to add the Dgraph Lambda server configuration, and map the JavaScript file that contains the code for lambda functions to the `/app/script/script.js` file. Remember to set the `DGRAPH_URL` environment variable to your Alpha server.

For example:

```yml
  lambda:
    image: dgraph/dgraph-lambda:latest
    container_name: lambda
    labels:
      cluster: test
    ports:
      - 8686:8686
    depends_on:
      - alpha
    environment:
      DGRAPH_URL: http://alpha:8180
    volumes:
      - type: bind
        source: ./script.js
        target: /app/script/script.js
        read_only: true
```

Here's a complete Docker example, including:

- Zero 
- Alpha
- Lambda

```yml
version: "3.5"
services:
  zero:
    image: dgraph/dgraph:latest
    container_name: zero1
    working_dir: /data/zero1
    ports:
      - 5180:5180
      - 6180:6180
    labels:
      cluster: test
      service: zero1
    volumes:
      - type: bind
        source: $GOPATH/bin
        target: /gobin
        read_only: true
    command: /gobin/dgraph zero -o 100 --logtostderr -v=2 --bindall --expose_trace --profile_mode block --block_rate 10 --my=zero1:5180

  alpha:
    image: dgraph/dgraph:latest
    container_name: alpha1
    working_dir: /data/alpha1
    volumes:
      - type: bind
        source: $GOPATH/bin
        target: /gobin
        read_only: true
    ports:
      - 8180:8180
      - 9180:9180
    labels:
      cluster: test
      service: alpha1
    command: /gobin/dgraph alpha --zero=zero1:5180 -o 100 --expose_trace --trace 1.0
      --profile_mode block --block_rate 10 --logtostderr -v=2
      --whitelist 10.0.0.0/8,172.16.0.0/12,192.168.0.0/16 --my=alpha1:7180
      --graphql_lambda_url=http://lambda:8686/graphql-worker

  lambda:
    image: dgraph/dgraph-lambda:latest
    container_name: lambda
    labels:
      cluster: test
    ports:
      - 8686:8686
    depends_on:
      - alpha
    environment:
      DGRAPH_URL: http://alpha:8180
    volumes:
      - type: bind
        source: ./script.js
        target: /app/script/script.js
        read_only: true
```
