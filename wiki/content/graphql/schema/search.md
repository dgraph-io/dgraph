+++
title = "Search and Filtering"
weight = 5
[menu.main]
    parent = "schema"
    identifier = "schema-search"
+++

The `@search` directive tells Dgraph what search to build into your GraphQL API.

When a type contains an `@search` directive, Dgraph constructs a search input type and a query in the GraphQL `Query` type. For example, if the schema contains

```graphql
type Post {
    ...
}
```

then Dgraph constructs a `queryPost` GraphQL query for querying posts.  The `@search` directives in the `Post` type control how Dgraph builds indexes and what kinds of search it builds into `queryPost`.  If the type contains

```graphql
type Post {
    ...
    datePublished: DateTime @search
}
```

then it's possible to filter posts with a date-time search like:

```graphql
query {
    queryPost(filter: { datePublished: { ge: "2020-06-15" }}) {
        ...
    }
}
```

If the type tells Dgraph to build search capability based on a term (word) index for the `title` field

```graphql
type Post {
    ...
    title: String @search(by: [term])
}
```

then, the generated GraphQL API will allow search by terms in the title.

```graphql
query {
    queryPost(filter: { title: { anyofterms: "GraphQL" }}) {
        ...
    }
}
```

Dgraph also builds search into the fields of each type, so searching is available at deep levels in a query.  For example, if the schema contained these types

```graphql
type Post {
    ...
    title: String @search(by: [term])
}

type Author {
    name: String @search(by: [hash])
    posts: [Post]
}
```

then Dgraph builds GraphQL search such that a query can, for example, find an author by name (from the hash search on `name`) and return only their posts that contain the term "GraphQL".

```graphql
queryAuthor(filter: { name: { eq: "Diggy" } } ) {
    posts(filter: { title: { anyofterms: "GraphQL" }}) {
        title
    }
}
```

There's different search possible for each type as explained below.

### Int, Float and DateTime

| argument | constructed filter |
|----------|----------------------|
| none | `lt`, `le`, `eq`, `ge` and `gt` |

Search for fields of types `Int`, `Float` and `DateTime` is enabled by adding `@search` to the field with no arguments.  For example, if a schema contains:

```graphql
type Post {
    ...
    numLikes: Int @search
}
```

Dgraph generates search into the API for `numLikes` in two ways: a query for posts and field search on any post list.

A field `queryPost` is added to the `Query` type of the schema.

```graphql
type Query {
    ...
    queryPost(filter: PostFilter, order: PostOrder, first: Int, offset: Int): [Post]
}
```

`PostFilter` will contain less than `lt`, less than or equal to `le`, equal `eq`, greater than or equal to `ge` and greater than `gt` search on `numLikes`.  Allowing for example:

```graphql
query {
    queryPost(filter: { numLikes: { gt: 50 }}) { 
        ... 
    }
}
```

Also, any field with a type of list of posts has search options added to it. For example, if the input schema also contained:

```graphql
type Author {
    ...
    posts: [Post]
}
```

Dgraph would insert search into `posts`, with

```graphql
type Author {
    ...
    posts(filter: PostFilter, order: PostOrder, first: Int, offset: Int): [Post]
}
```

That allows search within the GraphQL query.  For example, to find Diggy's posts with more than 50 likes.

```graphql
queryAuthor(filter: { name: { eq: "Diggy" } } ) {
    ...
    posts(filter: { numLikes: { gt: 50 }}) {
        title
        text
    }
}
```

### DateTime

| argument | constructed filters |
|----------|----------------------|
| `year`, `month`, `day`, or `hour` | `lt`, `le`, `eq`, `ge` and `gt` |

As well as `@search` with no arguments, `DateTime` also allows specifying how the search index should be built: by year, month, day or hour.  `@search` defaults to year, but once you understand your data and query patterns, you might want to changes that like `@search(by: [day])`.

### Boolean

| argument | constructed filter |
|----------|----------------------|
| none | `true` and `false` |

Booleans can only be tested for true or false.  If `isPublished: Boolean @search` is in the schema, then the search allows

```graphql
filter: { isPublished: true }
```

and

```graphql
filter: { isPublished: false }
```

### String

Strings allow a wider variety of search options than other types.  For strings, you have the following options as arguments to `@search`.

| argument | constructed searches |
|----------|----------------------|
| `hash` | `eq` |
| `exact` | `lt`, `le`, `eq`, `ge` and `gt` (lexicographically) |
| `regexp` | `regexp` (regular expressions) |
| `term` | `allofterms` and `anyofterms` |
| `fulltext` | `alloftext` and `anyoftext` |

* *Schema rule*: `hash` and `exact` can't be used together.

#### String exact and hash search

Exact and hash search has the standard lexicographic meaning. 

```graphql
query {
    queryAuthor(filter: { name: { eq: "Diggy" } }) { ... }
}
```

And for exact search

```graphql
query {
    queryAuthor(filter: { name: { gt: "Diggy" } }) { ... }
}
```

to find users with names lexicographically after "Diggy".

#### String regular expression search

Search by regular expression requires bracketing the expression with `/` and `/`.  For example, query for "Diggy" and anyone else with "iggy" in their name:

```graphql
query {
    queryAuthor(filter: { name: { regexp: "/.*iggy.*/" } }) { ... }
}
```

#### String term and fulltext search

If the schema has 

```graphql
type Post {
    title: String @search(by: [term])
    text: String @search(by: [fulltext])
    ...
}
```

then 

```graphql
query {
    queryPost(filter: { title: { `allofterms: "GraphQL tutorial"` } } ) { ... }
}
```

will match all posts with both "GraphQL and "tutorial" in the title, while `anyofterms: "GraphQL tutorial"` would match posts with either "GraphQL" or "tutorial".

`fulltext` search is Google-stye text search with stop words, stemming. etc.  So `alloftext: "run woman"` would match "run" as well as "running", etc.  For example, to find posts that talk about fantastic GraphQL tutorials:

```graphql
query {
    queryPost(filter: { title: { `alloftext: "fantastic GraphQL tutorials"` } } ) { ... }
}
```

#### Strings with multiple searches

It's possible to add multiple string indexes to a field.  For example to search for authors by `eq` and regular expressions, add both options to the type definition, as follows.

```graphql
type Author {
    ...
    name: String! @search(by: [hash, regexp])
}
```

### Enums

| argument | constructed searches |
|----------|----------------------|
| none | `eq` |
| `hash` | `eq` |
| `exact` | `lt`, `le`, `eq`, `ge` and `gt` (lexicographically) |
| `regexp` | `regexp` (regular expressions) |

Enums are serialized in Dgraph as strings.  `@search` with no arguments is the same as `@search(by: [hash])` and provides only `eq` search.  Also available for enums are `exact` and `regexp`.  For hash and exact search on enums, the literal enum value, without quotes `"..."`, is used, for regexp, strings are required. For example:

```graphql
enum Tag {
    GraphQL
    Database
    Question
    ...
}

type Post {
    ...
    tags: [Tag!]! @search
}
```

would allow

```graphql
query {
    queryPost(filter: { tags: { eq: GraphQL } } ) { ... }
}
```

Which would find any post with the `GraphQL` tag.

While `@search(by: [exact, regexp]` would also admit `lt` etc. and 

```graphql
query {
    queryPost(filter: { tags: { regexp: "/.*aph.*/" } } ) { ... }
}
```

which is helpful for example if the enums are something like product codes where regular expressions can match a number of values. 
