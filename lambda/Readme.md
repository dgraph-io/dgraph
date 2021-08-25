# Dgraph Lambda

Dgraph Lambda is a serverless platform for running JS on Dgraph Cloud.

## Running a script

A script looks something like this. There are two ways to add a resolver
* `addGraphQLResolver` which recieves `{ parent, args }` and returns a single value
* `addMultiParentGraphQLResolver` which received `{ parents, args }` and should return an array of results, each result matching to one parent. This method will have much better performance if you are able to club multiple requests together

If the query is a root query/mutation, parents will be set to `[null]`.

```javascript
const fullName = ({ parent: { firstName, lastName } }) => `${firstName} ${lastName}`

async function todoTitles({ graphql }) {
  const results = await graphql('{ queryTodo { title } }')
  return results.data.queryTodo.map(t => t.title)
}

self.addGraphQLResolvers({
  "User.fullName": fullName,
  "Query.todoTitles": todoTitles,
})

async function reallyComplexDql({parents, dql}) {
  const ids = parents.map(p => p.id);
  const someComplexResults = await dql(`really-complex-query-here with ${ids}`);
  return parents.map(parent => someComplexResults[parent.id])
}

self.addMultiParentGraphQLResolvers({
  "User.reallyComplexProperty": reallyComplexDql
})
```

## Running Locally [Needs to be updated]

First launch dgraph and load it with the todo schema (and add a todo or two).

```graphql
type User {
   id: ID!
   firstName: String!
   lastName: String!
   fullName: String @lambda
}

type Todo {
   id: ID!
   title: String
}

type Query {
  todoTitles: [String] @lambda
}
```

```bash
# host.docker.internal may not work on old versions of docker
docker run -it --rm -p 8686:8686 -v /path/to/script.js:/app/script/script.js -e DGRAPH_URL=http://host.docker.internal:8080 dgraph/dgraph-lambda
```

Note for linux: host.docker.internal doesn't work on older versions of docker on linux. You can use `DGRAPH_URL=http://172.17.0.1:8080` instead

Then test it out with the following curls
```bash
curl localhost:8686/graphql-worker -H "Content-Type: application/json" -d '{"resolver":"User.fullName","parents":[{"firstName":"Dgraph","lastName":"Labs"}]}'
```

## Environment

We are trying to make the environment match the environment you'd get from ServiceWorker.

* [x] fetch
* [x] graphql / dql
* [x] base64
* [x] URL
* [ ] crypto - should test this

## Adding libraries

If you would like to add libraries, then use webpack --target=webworker to compile your script. We'll fill out these instructions later.

### Working with Typescript

You can import @slash-graph/lambda-types to get types for `addGraphQLResolver` and `addGraphQLMultiParentResolver`.

## Security

Currently, this uses node context to try and make sure that users aren't up to any fishy business. However, contexts aren't true security, and we should eventually switch to isolates.

## Publishing [Needs to be updated]

Currently, the publishing of this isn't automated. In order to publish:
* Publish the types in lambda-types if needed with (npm version minor; npm publish)
* The docker-image auto publishes, but pushing a tag will create a tagged version that is more stable
