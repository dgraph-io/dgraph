# Local Test

A collection of make commands that enable:

- hot reloading of built local images in a docker compose environment
- creating/updating dql/graphql schemas
- loading data in RDF and JSON encoding
- running DQL/GraphQL queries/mutations

Requirements

- Docker
- make
- curl
- [jq](https://stedolan.github.io/jq/download/) (optional, for formatting JSON results)
- [gql](https://github.com/matthewmcneely/gql/tree/feature/add-query-and-variables-from-file/builds)
  (for running graphql queries)

One final requirement is to build a local image of dgraph from the source currently on your machine.

```bash
cd .. && make image-local
```

This will build a `dgraph/dgraph:local` image in your local Docker registry.

## Make targets

### `make help`

Lists all available make targets and a short description.

### `make up`

Brings up a simple alpha and zero node using docker compose in your local Docker environment. The
target then tails the log out from both containers. This target also launches a
_[watchtower](https://containrrr.dev/watchtower/)_ container that will automatically restart alpha
and zero when it detects a new dgraph image (built via `cd .. && make image-local`).

The process for hot-reloading development basically involves `make up`, modifying source on your
machine, then `make image-local`. The changes in your source will show up in the locally deployed
dgraph containers when watchtower restarts them.

Note that this deployment is completely insecure—it's meant for local testing only.

### `make up-with-lambda`

Brings up the alpha and zero containers along with the dgraph lambda container. Note this lambda
container is based on `dgraph/dgraph-lambda:latest`. If you're trying to debug the lambda container,
you'll need reference your local image in the docker compose file.

### `make down` and `make down-with-lambda`

Stops the containers.

### `make refresh`

Restarts the containers if a new `dgraph/dgraph:local` image is available. This shouldn't be needed
if the _watchtower_ container is running correctly.

### `make schema-dql`

Updates dgraph with the schema defined in `schema.dql`.

Example schema.dql:

```dql
type Person {
    name
    boss_of
    works_for
}

type Company {
    name
    industry
    work_here
}

industry: string @index(term) .
boss_of: [uid] .
name: string @index(exact, term) .
work_here: [uid] .
boss_of: [uid] @reverse .
works_for: [uid] @reverse .
```

### `make schema-gql`

Updates dgraph with the schema defined in `schema.gql`

Example schema.gql:

```graphql
type Post {
  id: ID!
  title: String!
  text: String
  datePublished: DateTime
  author: Author!
}

type Author {
  id: ID!
  name: String!
  posts: [Post!] @hasInverse(field: author)
}
```

### `make drop-data`

Drops all data from the cluster, but not the schema.

### `make drop-all`

Drops all data and the schema from the cluster.

### `make load-data-gql`

Loads JSON data defined in `gql-data.json`. This target is useful for loading data into schemas
defined with GraphQL SDL.

Example gql-data.json:

```json
[
  {
    "uid": "_:katie_howgate",
    "dgraph.type": "Author",
    "Author.name": "Katie Howgate",
    "Author.posts": [
      {
        "uid": "_:katie_howgate_1"
      },
      {
        "uid": "_:katie_howgate_2"
      }
    ]
  },
  {
    "uid": "_:timo_denk",
    "dgraph.type": "Author",
    "Author.name": "Timo Denk",
    "Author.posts": [
      {
        "uid": "_:timo_denk_1"
      },
      {
        "uid": "_:timo_denk_2"
      }
    ]
  },
  {
    "uid": "_:katie_howgate_1",
    "dgraph.type": "Post",
    "Post.title": "Graph Theory 101",
    "Post.text": "https://www.lancaster.ac.uk/stor-i-student-sites/katie-howgate/2021/04/27/graph-theory-101/",
    "Post.datePublished": "2021-04-27",
    "Post.author": {
      "uid": "_:katie_howgate"
    }
  },
  {
    "uid": "_:katie_howgate_2",
    "dgraph.type": "Post",
    "Post.title": "Hypergraphs – not just a cool name!",
    "Post.text": "https://www.lancaster.ac.uk/stor-i-student-sites/katie-howgate/2021/04/29/hypergraphs-not-just-a-cool-name/",
    "Post.datePublished": "2021-04-29",
    "Post.author": {
      "uid": "_:katie_howgate"
    }
  },
  {
    "uid": "_:timo_denk_1",
    "dgraph.type": "Post",
    "Post.title": "Polynomial-time Approximation Schemes",
    "Post.text": "https://timodenk.com/blog/ptas/",
    "Post.datePublished": "2019-04-12",
    "Post.author": {
      "uid": "_:timo_denk"
    }
  },
  {
    "uid": "_:timo_denk_2",
    "dgraph.type": "Post",
    "Post.title": "Graph Theory Overview",
    "Post.text": "https://timodenk.com/blog/graph-theory-overview/",
    "Post.datePublished": "2017-08-03",
    "Post.author": {
      "uid": "_:timo_denk"
    }
  }
]
```

### `make load-data-dql-json`

Loads JSON data defined in `dql-data.json`. This target is useful for loading data into schemas
defined with base dgraph types.

Example dql-data.json:

```json
{
  "set": [
    {
      "uid": "_:company1",
      "industry": "Machinery",
      "dgraph.type": "Company",
      "name": "CompanyABC"
    },
    {
      "uid": "_:company2",
      "industry": "High Tech",
      "dgraph.type": "Company",
      "name": "The other company"
    },
    {
      "uid": "_:jack",
      "works_for": { "uid": "_:company1" },
      "dgraph.type": "Person",
      "name": "Jack"
    },
    {
      "uid": "_:ivy",
      "works_for": { "uid": "_:company1" },
      "boss_of": { "uid": "_:jack" },
      "dgraph.type": "Person",
      "name": "Ivy"
    },
    {
      "uid": "_:zoe",
      "works_for": { "uid": "_:company1" },
      "dgraph.type": "Person",
      "name": "Zoe"
    },
    {
      "uid": "_:jose",
      "works_for": { "uid": "_:company2" },
      "dgraph.type": "Person",
      "name": "Jose"
    },
    {
      "uid": "_:alexei",
      "works_for": { "uid": "_:company2" },
      "boss_of": { "uid": "_:jose" },
      "dgraph.type": "Person",
      "name": "Alexei"
    }
  ]
}
```

### `make load-data-dql-rdf`

Loads RDF data defined in `dql-data.rdf`. This target is useful for loading data into schemas
defined with base dgraph types.

Example dql-data.rdf:

```rdf
{
  set {
    _:company1 <name> "CompanyABC" .
    _:company1 <dgraph.type> "Company" .
    _:company2 <name> "The other company" .
    _:company2 <dgraph.type> "Company" .

    _:company1 <industry> "Machinery" .

    _:company2 <industry> "High Tech" .

    _:jack <works_for> _:company1 .
    _:jack <dgraph.type> "Person" .

    _:ivy <works_for> _:company1 .
    _:ivy <dgraph.type> "Person" .

    _:zoe <works_for> _:company1 .
    _:zoe <dgraph.type> "Person" .

    _:jack <name> "Jack" .
    _:ivy <name> "Ivy" .
    _:zoe <name> "Zoe" .
    _:jose <name> "Jose" .
    _:alexei <name> "Alexei" .

    _:jose <works_for> _:company2 .
    _:jose <dgraph.type> "Person" .
    _:alexei <works_for> _:company2 .
    _:alexei <dgraph.type> "Person" .

    _:ivy <boss_of> _:jack .

    _:alexei <boss_of> _:jose .
  }
}
```

### `make query-dql`

Runs the query defined in query.dql.

Example query.dql:

```dql
{
  q(func: eq(name, "CompanyABC")) {
    name
    works_here : ~works_for {
        uid
        name
    }
  }
}
```

### `make query-gql`

Runs the query defined in query.gql and optional variables defined in variables.json.

Example query-gql:

```graphql
query QueryAuthor($order: PostOrder) {
  queryAuthor {
    id
    name
    posts(order: $order) {
      id
      datePublished
      title
      text
    }
  }
}
```

Example variables.json:

```json
{
  "order": {
    "desc": "datePublished"
  }
}
```
