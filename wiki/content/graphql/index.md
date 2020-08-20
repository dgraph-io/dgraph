+++
title = "GraphQL"
+++

## Quick Start

Let's go from nothing to a running GraphQL API in just two steps.

For GraphQL in Dgraph, you just concentrate on defining the schema of your graph and how you'd like to search that graph; Dgraph does the rest.  You work only with GraphQL and, think in terms of the graph that matters for your app.

This example is for an app about customers, products and reviews.  That's a pretty simple graph, with just three types of objects, but it has some interesting connections for us to explore.

Here's a schema of GraphQL types for that:

```graphql
type Product {
    productID: ID!
    name: String @search(by: [term])
    reviews: [Review] @hasInverse(field: about)
}

type Customer {
    username: String! @id @search(by: [hash, regexp])
    reviews: [Review] @hasInverse(field: by)
}

type Review {
    id: ID!
    about: Product!
    by: Customer!
    comment: String @search(by: [fulltext])
    rating: Int @search
}
```

With Dgraph you can turn that schema into a running GraphQL API in just two steps.

## Step 1 - Start Dgraph GraphQL

It's a one-liner to bring up Dgraph with GraphQL.  *Note: The Dgraph standalone image is great for quick start and exploring, but it's not meant for production use.  Once you want to build an App or persist your data for restarts, you'll need to review the   [admin docs](/admin).*

```bash
docker run -it -p 8080:8080 dgraph/standalone:v20.03.1
```

With that, GraphQL has started at localhost:8080/graphql, but it doesn't have a schema to serve yet. 

## Step 2 - Add a GraphQL Schema

Dgraph will run your GraphQL API at `/graphql` and an admin interface at `/admin`.  The `/admin` interface lets you add and update the GraphQL schema served at `/graphql`.  The quickest way to reset the schema is just to post it to `/admin` with curl.  

Take the schema above, cut-and-paste it into a file called `schema.graphql` and run the following curl command.

```bash
curl -X POST localhost:8080/admin/schema --data-binary '@schema.graphql'
```

It'll post back the types it's currently serving a schema for, which should be the same as the input schema.

That's it, now you've got a GraphQL API up and running.

No, really, that's all; nothing else to do; it's there, serving GraphQL --- let's go use it.

## GraphQL Mutations

If you've followed the steps above, there's a GraphQL server up and running.  You can access that GraphQL endpoint with any of the great GraphQL developer tools.  Good choices include [GraphQL Playground](https://github.com/prisma-labs/graphql-playground), [Insomnia](https://insomnia.rest/), [GraphiQL](https://github.com/graphql/graphiql) and [Altair](https://github.com/imolorhe/altair).  

Fire one of those up and point it at `http://localhost:8080/graphql`.  If you know lots about GraphQL, you might want to explore the schema, queries and mutations that were generated from the input.

We'll begin by adding some products and an author.  GraphQL can accept multiple mutations at a time, so it's one request.  Neither the products nor the author will have any reviews yet, so all we need is the names.

```graphql
mutation {
  addProduct(input: [
    { name: "GraphQL on Dgraph"},
    { name: "Dgraph: The GraphQL Database"}
  ]) {
    product {
      productID
      name
    }
  }
  addCustomer(input: [{ username: "Michael"}]) {
    customer {
      username
    }
  }
}
```

The GraphQL server will return a json response like:

```json
{
  "data": {
    "addProduct": {
      "product": [
        {
          "productID": "0x2",
          "name": "GraphQL on Dgraph"
        },
        {
          "productID": "0x3",
          "name": "Dgraph: The GraphQL Database"
        }
      ]
    },
    "addCustomer": {
      "customer": [
        {
          "username": "Michael"
        }
      ]
    }
  },
  "extensions": {
    "requestID": "b155867e-4241-4cfb-a564-802f2d3808a6"
  }
}
```

And, of course, our author bought "GraphQL on Dgraph", loved it, and added a glowing review with the following mutation.  

Because the schema defined Customer with the field `username: String! @id`, the `username` field acts like an ID, so we can identify customers just with their names.  Products, on the other hand, had `productID: ID!`, so they'll get an auto-generated ID.  Your ID for the product might be different.  Make sure you check the result of adding the products and use the right ID -  it's no different to linking primary/foreign keys correctly in a relational DB.

```graphql
mutation {
  addReview(input: [{
    by: {username: "Michael"}, 
    about: { productID: "0x2"}, 
    comment: "Fantastic, easy to install, worked great.  Best GraphQL server available",
    rating: 10}]) 
  {
    review {
      comment
      rating
      by { username }
      about { name }
    }
  }
}
```

This time, the mutation result queries for the author making the review and the product being reviewed, so it's gone deeper into the graph to get the result than just the mutation data.

```json
{
  "data": {
    "addReview": {
      "review": [
        {
          "comment": "Fantastic, easy to install, worked great.  Best GraphQL server available",
          "rating": 10,
          "by": {
            "username": "Michael"
          },
          "about": {
            "name": "GraphQL on Dgraph"
          }
        }
      ]
    }
  },
  "extensions": {
    "requestID": "11bc2841-8c19-45a6-bb31-7c37c9b027c9"
  }
}
```

Already we have a running GraphQL API and can add data using any GraphQL tool.  You could write a GraphQL/React app with a nice UI.  It's GraphQL, so you can do anything GraphQL with your new server. 

Go ahead, add some more customers, products and reviews and then move on to querying data back out.

## GraphQL Queries

Mutations are one thing, but query is where GraphQL really shines.  With GraphQL, you get just the data you want, in a format that's suitable for your app.

With Dgraph, you get powerful graph search built into your GraphQL API.  The schema for search is generated from the schema document that we started with and automatically added to the GraphQL API for you.  

Remember the definition of a review.

```graphql
type Review {
    ...
    comment: String @search(by: [fulltext])
    ...
}
```

The directive `@search(by: [fulltext])` tells Dgraph we want to be able to search for comments with full-text search.  That's Google-style search like 'best buy' and 'loved the color'.  Dgraph took that, and the other information in the schema, and built queries and search into the API.

Let's find all the products that were easy to install.

```graphql
query {
  queryReview(filter: { comment: {alloftext: "easy to install"}}) {
    comment
    by {
      username
    }
    about {
      name
    }
  }
}
```

What reviews did you get back?  It'll depend on the data you added, but you'll at least get the initial review we added.  

Maybe you want to find reviews that describe best GraphQL products and give a high rating.

```graphql
query {
  queryReview(filter: { comment: {alloftext: "best GraphQL"}, rating: { ge: 10 }}) {
    comment
    by {
      username
    }
    about {
      name
    }
  }
}
```

How about we find the customers with names starting with "Mich" and the five products that each of those liked the most.

```graphql
query {
  queryCustomer(filter: { username: { regexp: "/Mich.*/" } }) {
    username
    reviews(order: { asc: rating }, first: 5) {
      comment
      rating
      about {
        name
      }
    }
  }
}
```

We started with nothing more than the definition of three GraphQL types, yet already we have a running GraphQL API that keeps usernames unique, can run queries and mutations, and we are on our way for an e-commerce app.  

There's much more that could be done: we can build in more types, more powerful search, build in queries that work through the graph like a recommendation system, and more.  Keep learning about GraphQL with Dgraph to find out about great things you can do.

## Where Next

Depending on if you need a bit more of a walkthrough or if you're off and running, you should checkout the worked example or the sample React app.

The worked example builds through similar material to this quick start, but also works through what's allowed in your input schema and what happens to what you put in there.

The React app is a UI for a simple social media example that's built on top of a Dgraph GraphQL instance.

Later, as you're building your app, you'll need the reference materials on schema and server administration.

## The API

How to use the GraphQL API. 

Dgraph serves [spec compliant
GraphQL](https://graphql.github.io/graphql-spec/June2018/) over HTTP. By
default, GraphQL is served at `http://localhost:8080/graphql`. Both GET and POST
requests are served.

For POST requests, the body must be "application/json" encoded with the following form.

```json
{
  "query": "...",
  "operationName": "...",
  "variables": { "var": "val", ... }
}
```

For GET requests, the query, variables and operation are sent as query parameters in the url.

```
http://localhost:8080/graphql?query={...}&variables={...}&operation=...
```

In both cases, only `query` is required. `variables` is only required if the query contains GraphQL variables: i.e. it starts like `query myQuery($var: String...)`. While, `operationName` is required if there are multiple operations in the query; in which case, operations must also be named.

Compressed requests and responses are available with gzip. Set header `Content-Encoding` to `gzip` to post encoded data, and `Accept-Encoding` to `gzip` to receive a compressed response.

When an operation contains multiple queries, they are run concurrently and independently in a single Dgraph transaction.

When an operation contains multiple mutations, they are run serially, in the order listed in the request, and in a transaction per mutation. If a mutation fails, the following mutations are not executed, and previous mutations are not rolled back.

## Example

A worked example of how to build a GraphQL API for an App.

# GraphQL, Dgraph and Graphs

You're familiar with GraphQL types, fields and resolvers.  Maybe you've written an app that adds GraphQL over a REST endpoint or maybe over a relational database.  So you know how GraphQL sits over those sources and issues many queries to translate the REST/relational data into something that looks like a graph.  

You know there's a cognitive jump because your app is about a graph, but you've got to design a relational schema and work out how that translates as a graph; you'll think about the app in terms of the graph, but always have to mentally translate back and forth between the relational and graph models.  There are engineering challenges around the translation as well as the efficiency of the queries.  

There's none of that here.  

Dgraph GraphQL is part of Dgraph, which stores a graph - it's a database of nodes and edges.  So it's efficient to store, query and traverse as a graph.  Your data will get stored just like you design it in the schema, and the queries are a single graph query that does just what the GraphQL query says.


## How it Works

With Dgraph you design your application in GraphQL.  You design a set of GraphQL types that describes your requirements.  Dgraph takes those types, prepares graph storage for them and generates a GraphQL API with queries and mutations.

You design a graph, store a graph and query a graph.  You think and design in terms of the graph that your app is based around.

Let's move on to the design process - it's graph-first, in fact, it's GraphQL-first.  We'll design the GraphQL types that our example app is based around, and ... we'll there's no *and* ... from that, you get a GraphQL API for those types; you just move on to building the app around it.

## An Example App

Say you are working to build a social media app.  In the app, there are authors writing questions and others answering or making comments to form conversation threads.  You'll want to render things like a home page for each author as well as a feed of interesting posts or search results.  Maybe authors can subscribe to search terms or tags that interest them.  Navigating to a post renders the initial post as well as the following conversation thread.

## Your First Schema

Here we'll design a schema for the app and, in the next section, turn that into a running GraphQL API with Dgraph.

It's version 0.1 of the app, so let's start small. For the first version, we are interested in an author's name and the list of things they've posted.  That's a pretty simple GraphQL type.

```graphql
type Author {
  username: String!
  posts: [Post] 
}
```

But defining questions and answers is a little trickier.  We could define `type Question { ... }` and `type Answer { ... }`, but then how would comments work.  We want to be able to comment on questions and comment on answers and even have comment threads: so comments that comment on comments.  

We also want to cut down on the amount of boilerplate we need to write, so it's great if we don't need to say that there's an author for a question, and an author for an answer and an author for a comment - see it gets repetitive!

GraphQL has interfaces that solve this problem, and Dgraph lets you use them in a way that cuts down on repetition.

Let's have a GraphQL interface that collects together the common data for the types of text the user can post on the site.  We'll need the author who posted as well as the actual text and the date.

```graphql
interface Post {
  id: ID!
  text: String 
  datePublished: DateTime 
  author: Author!
}
```

Questions are a kind of post (that is, questions have `text`, a `datePublished` and were published by an `author`) that also have a list of answers.  Answers themselves answer a particular question (as well as having the data of a post).  And comments can comment on any kind of post: questions, answers or other comments.

```graphql
type Question implements Post {
  answers: [Answer]
}

type Answer implements Post {
  inAnswerTo: Question!
}

type Comment implements Post {
  commentsOn: Post!
}
```

Given that, we care about more than just the posts an author has made.  Let's say we want to know the questions and answers a user has posted.

```graphql
type Author {
  username: String!
  questions: [Question] 
  answers: [Answer]
}
```

## More than Types

That's enough to describe the types, but, even in the first version, we'll want some ways to search the data - how else could I find the newest 10 posts about "GraphQL".  

Dgraph allows adding extra declarative specifications to the schema file, and it uses these to interpret the schema in particular ways or to add features to the GraphQL API it generates.

Adding the directive `@search` tells Dgraph what fields you want to search by.  The post text is an obvious candidate.  We'll want to search that Google-style, with a search like "GraphQL introduction tutorial".  That's a full-text search.  We can let the API know that's how we'd like to search posts text by updating the schema with:

```graphql
interface Post {
  ...
  text: String @search(by: [fulltext])
  ...
}
``` 

Let's say we also want to find authors by name.  A hash-based search is pretty good for that.  

```graphql
type Author {
  ...
  username: String! @search(by: [hash])
  ...
}
```

We'll also add less-than, greater-than-or-equal-to date searching on `datePublished` - no arguments required to `@search` this time.

```graphql
interface Post {
  ... 
  datePublished: DateTime @search
  ...
}
```

With those directives in the schema, Dgraph will build search capability into our GraphQL API.

We also want to make sure that usernames are unique.  The `@id` directive takes care of that - it also automatically adds hash searching, so we can drop the `@search(by: [hash])`, though having it also causes no harm.

```graphql
type Author {
  username: String! @id
  ...
}
```

Now the GraphQL API will ensure that usernames are unique and will build search and mutation capability such that a username can be used like an identifier/key.  The `id: ID!` in `Post` means that an auto-generated ID will be used to identify posts.

The only remaining thing is to recognize how GraphQL handles relations. So far, our GraphQL schema says that an author has some questions and answers and that a post has an author, but the schema doesn't connect them as a two-way edge in the graph: e.g. it doesn't say that the questions I can reach from a particular author all have that author as their author.  

GraphQL schemas are always under-specified in this way. It's left up to the documentation and implementation to make the two-way connection, if it exists.  Here, we'll make sure they hook up in the right way by adding the directive `@hasInverse`.

Here it is in the complete GraphQL schema.

```graphql
type Author {
  username: String! @id
  questions: [Question] @hasInverse(field: author)
  answers: [Answer] @hasInverse(field: author)
}

interface Post {
  id: ID!
  text: String! @search(by: [fulltext])
  datePublished: DateTime @search
  author: Author!
  comments: [Comment] @hasInverse(field: commentsOn)
}

type Question implements Post {
  answers: [Answer] @hasInverse(field: inAnswerTo)
}

type Answer implements Post {
  inAnswerTo: Question!
}

type Comment implements Post {
  commentsOn: Post!
}
```

# Running

Starting Dgraph with GraphQL can be done by running from the all-in-one docker image.  *Note: The Dgraph standalone image is great for quick start and exploring, but it's not meant for production use.  Once you want to build an App or persist your data for restarts, you'll need to review the  [admin docs](/admin).*

```bash
docker run -it -p 8080:8080 dgraph/standalone:v20.03.1
```

That brings Dgraph and enables GraphQL at `localhost:8080`.  Dgraph serves two GraphQL endpoints: 

* at `/graphql` it serves the GraphQL API for your schema; 
* at `/admin` it serves a GraphQL schema for administering your system.  

We'll use the mutation `updateGQLSchema` at the `/admin` service to add the GraphQL schema and refresh what's served at `/graphql`. 

Take the schema above, cut-and-paste it into a file called `schema.graphql` and run the following curl command.

```bash
curl -X POST localhost:8080/admin/schema --data-binary '@schema.graphql'
```

Now Dgraph is serving a GraphQL schema for the types we defined.

# Introspection

So we've taken the input types and generated a running GraphQL API, but what's in the API?

The API responds to GraphQL schema introspection, so you can consume it with anything that's GraphQL: e.g. [GraphQL Playground](https://github.com/prisma-labs/graphql-playground), [Insomnia](https://insomnia.rest/), [GraphiQL](https://github.com/graphql/graphiql) and [Altair](https://github.com/imolorhe/altair).  

Point your favorite tool at `http://localhost:8080/graphql` and schema introspection will show you what's been generated.

Rather than digging through everything that was generated, let's explore it by running some mutations and queries.

# Mutations

For each type in the input types, Dgraph generated add, update and delete mutations.

Adding authors and posts is the place to start with mutations.

The generated GraphQL API contains:

```graphql
type Mutation {
  ...
  addAuthor(input: AddAuthorInput): AddAuthorPayload
  ...
}
``` 

The input type `AddAuthorInput` really just requires a name for the author.  The add mutation can add multiples, so we can add some authors with:

```graphql
mutation {
  addAuthor(input: [
    { username: "Michael" },
    { username: "Apoorv" }
  ]) {
    author {
      username
    }
  }
}
``` 

Which will return

```json
{
  "data": {
    "addAuthor": {
      "author": [
        {
          "username": "Michael"
        },
        {
          "username": "Apoorv"
        }
      ]
    }
  }
}
```

The schema specified those usernames as `@id`, so Dgraph makes sure that they are unique and you'll get an error if you try to add an author with a username that already exists (you can update an existing author with the `updateAuthor` mutation).

The generated GraphQL also contains a mutation for adding questions:

```graphql
type Mutation {
  ...
  addQuestion(input: AddQuestionInput): AddQuestionPayload
  ...
}
``` 

To add a question, you'll need to link it up to the right author, which you can do using its id - the username.  Of course, you can use GraphQL variables to supply the data.

```graphql
mutation addQuestion($question: AddQuestionInput!){
  addQuestion(input: [$question]) {
    question {
      id
      text
      datePublished
      author {
        username
      }
    }
  }
}
```

With variables

```json
{
  "question": {
    "datePublished": "2019-10-30",
    "text": "The very fist post about GraphQL in Dgraph.",
    "author": { "username": "Michael" }
  }
}
```

That will return something like.

```json
{
  "data": {
    "addQuestion": {
      "question": [
        {
          "id": "0x4",
          "text": "The very fist post about GraphQL in Dgraph.",
          "datePublished": "2019-10-30T00:00:00Z",
          "author": {
            "username": "Michael"
          }
        }
      ]
    }
  }
}
```

Authors can comment on posts, so let's also add a comment on that post.  

```graphql
mutation addComment($comment: AddCommentInput!){
  addComment(input: [$comment]) {
    comment {
      id
      text
      datePublished
      author {
        username
      }
      commentsOn {
        text
        author {
          username
        }
      }
    }
  }
}
```

Because posts have an auto generated `ID`, you need to make sure you link to the right post in the following variables.

```json
{
  "comment": {
    "datePublished": "2019-10-30",
    "text": "Wow, great work.",
    "author": { "username": "Apoorv" },
    "commentsOn": { "id": "0x4" }
  }
}
```

The mutation asks for more than just the mutated data, so the response digs deeper into the graph and finds the text of the post being commented on and the author.

```json
{
  "data": {
    "addComment": {
      "comment": [
        {
          "id": "0x5",
          "text": "Wow, great work.",
          "datePublished": "2019-10-30T00:00:00Z",
          "author": {
            "username": "Apoorv"
          },
          "commentsOn": {
            "text": "The very fist post about GraphQL in Dgraph.",
            "author": {
              "username": "Michael"
            }
          }
        }
      ]
    }
  }
}
```

Mutations don't have to be just one new object, or just linking to existing objects.  A mutation can also add deeply nested data.  Let's add a new author and their first question as a single mutation.

```graphql
mutation {
  addAuthor(input: [
    { 
      username: "Karthic",
      questions: [
        {
          datePublished: "2019-10-30",
          text: "How do I add nested data?"  
        }
      ]
    }
  ]) {
    author {
      username
      questions {
        id
        text
      }
    }
  }
}
```

We don't need say who the author of the question is this time - Dgraph works it out from the `@hasInverse` directive in the schema.

```json
{
  "data": {
    "addAuthor": {
      "author": [
        {
          "username": "Karthic",
          "questions": [
            {
              "id": "0x6",
              "text": "How do I add nested data?"
            }
          ]
        }
      ]
    }
  }
}
```

Notice how the structure of the input data for a mutation is just what you'd have as an object model in your app.  There's no special edges, no internal "add", or "link" in those deep mutations.  You don't have to build a special object to make mutations; you can just serialize the model you are using in your program and send it back to Dgraph.  

It even works if you send too much data.  Let's say your app is making an update where an author is answering the question above.  It'll use the `addAnswer` mutation.  

```graphql
mutation addAnswer($answer: AddAnswerInput!){
  addAnswer(input: [$answer]) {
    answer {
      id
      text
    }
  }
}
```

In your app you've got the original question, you've built the answer and linked them in whatever way is right in your programming language, but when you serialize the answer, you'll get.

```json
{
  "answer": {
    "text": "Don't worry deep mutations just work",
    "author": { "username": "Michael" },
    "inAnswerTo": { 
      "id": "0x6",
      "text": "How do I add nested data?" 
    }
  }
}
```

It doesn't matter that the question data is repeated.  Dgraph works out that "0x6" is an existing post and links to it without trying to alter its existing contents.  So you can just serialize your client side data and you don't even have to strip out the extra data when linking to existing objects.

Play around with it for a bit - add some authors and posts; there's also update and delete mutations you'll find by inspecting the schema.  Next, we'll see how to query data. 

# Queries

For each type in the input schema, two kinds of queries get generated.

```graphql
type Query {
  ...
  getAuthor(username: String!): Author
  getPost(id: ID!): Post
  ...
  queryAuthor(filter: AuthorFilter, order: AuthorOrder, first: Int, offset: Int): [Author]
  queryPost(filter: PostFilter, order: PostOrder, first: Int, offset: Int): [Post]
  ...
}
```

The get queries grab a single object by ID, while query is where Dgraph added the search capability it built from the `@search` directives in the schema.

Because the username is an author's ID, `getAuthor` takes as input the username to find.  Posts use the auto generated ID and so `getPost` takes that as input.

The filters in `AuthorFilter` and `PostFilter` are generated depending on what fields had an `@search` directive in the schema.  The possible orderings in `order` are worked out from the types of the fields.  And `first` and `offset` let you paginate results.

Getting an author by their id is just

```graphql
query {
  getAuthor(username: "Karthic") { 
    username 
    questions { text }
  }
}
```

For a post it's

```graphql
query {
  getPost(id: "0x4") {
    text
    author {
      username
    }
  }
}
```

Query `queryAuthor` works by applying any `filter`, `order` or pagination, and if none are given, it's just a search for things of that type.  For example, get all authors with:

```graphql
query {
  queryAuthor {
    username
    answers {
      text
    }
    questions {
      text
    }
  }
}
```

Or sort the authors alphabetically by name and get the first 5.

```graphql
query {
  queryAuthor(order: { asc: username }, first: 5) {
    username
  }
}
```

More interesting is querying posts.  In the app, you'd perhaps add a search field to the UI and maybe allow search for matching questions.  Here's how you'd get the latest 10 questions that mention GraphQL.

```graphql
query {
  queryPost(filter: { text: { anyoftext: "GraphQL"}}, order: { desc: datePublished }, first: 10) {
    text
    author {
      username
    }
  }
}
```

The query options also work deeper in queries.  So you can, for example, also find the most recent post of each author.

```graphql
query {
  queryPost(filter: { text: { anyoftext: "GraphQL"}}, order: { desc: datePublished }, first: 10) {
    text
    author {
      username
      questions(order: { desc: datePublished }, first: 1) {
        text
        datePublished
      }
    }
  }
}
```

# Updating the App

You've got v1 of your App working.  So you'll start iterating on your design an improving it.  Now you want authors to be able to tag questions and search for questions that have particular tags.

Dgraph makes this easy.  You can just update your schema and keep working.

That'll be updating the definition of Question to

```graphql
type Question implements Post {
  answers: [Answer]
  tags: [String!] @search(by: [term])
}
```

So the full schema becomes

```graphql
type Author {
  username: String! @id @search(by: [hash])
  questions: [Question] @hasInverse(field: author)
  answers: [Answer] @hasInverse(field: author)
}

interface Post {
  id: ID!
  text: String @search(by: [fulltext])
  datePublished: DateTime @search
  author: Author!
}

type Question implements Post {
  answers: [Answer]
  tags: [String!] @search(by: [term])
}

type Answer implements Post {
  inAnswerTo: Question!
}

type Comment implements Post {
  commentsOn: Post!
}
```

Update the schema as you did before and Dgraph will adjust to the new schema.

But all those existing questions won't have tags, so let's add some.  How about we tag every question that contains "GraphQL" with the tag "graphql".  The `updateQuestion` mutation allows us to filter for the questions we want to update and then either `set` new values or `remove` existing values.

The same filters that work for queries and mutations (update and delete) also work in mutation results, so we can update all the matching question to have the "graphql" tag, while returning a result that only contains the most recent such questions.

```graphql
mutation {
  updateQuestion(input: {
    filter: { text: { anyoftext: "GraphQL" }},
    set: { tags: ["graphql"]}
  }) {
    question(order: { desc: datePublished }, first: 10) {
      text
      datePublished
      tags
      author {
        username
      }
    }
  }
}
```

## Schema

All the things you can put in your input GraphQL schema, and what gets generated from that.

The process for serving GraphQL with Dgraph is to add a set of GraphQL type definitions using the `/admin` endpoint.  Dgraph takes those definitions, generates queries and mutations, and serves the generated GraphQL schema.  

The input schema may only contain interfaces, types and enums that follow the usual GraphQL syntax and validation rules.  Additional validation rules are called out below.

If you want to make your schema editing experience nicer, you should use an editor that does syntax highlighting for GraphQL.  With that, you may also want to include the definitions [here](#schemafragment) as an import.

# <a name="Scalars"></a>Scalars

Dgraph GraphQL comes with the standard GraphQL scalars: `Int`, `Float`, `String`, `Boolean` and `ID`.  There's also a `DateTime` scalar - represented as a string in RFC3339 format.

Scalars `Int`, `Float`, `String` and `DateTime` can be used in lists.  All scalars may be nullable or non-nullable.

The `ID` type is special.  IDs are auto-generated, immutable, and can be treated as strings.  Fields of type `ID` can be listed as nullable in a schema, but Dgraph will never return null. 

* *Schema rule*: `ID` lists aren't allowed - e.g. `tags: [String]` is valid, but `ids: [ID]` is not.
* *Schema rule*: Each type you define can have at most one field with type `ID`.  That includes IDs implemented through interfaces.

It's not possible to define further scalars - you'll receive an error if the input schema contains the definition of a new scalar.

For example, the following GraphQL type uses all of the available scalars.

```graphql
type User {
    userID: ID!
    name: String!
    lastSignIn: DateTime
    recentScores: [Float]
    reputation: Int
    active: Boolean
}
```

Scalar lists in Dgraph act more like sets, so `tags: [String]` would always contain unique tags.  Similarly, `recentScores: [Float]` could never contain duplicate scores.

# <a name="Enums"></a>Enums

You can define enums in your input schema.  For example:

```graphql
enum Tag {
    GraphQL
    Database
    Question
    ...
}

type Post {
    ...
    tags: [Tag!]!
}
```

# <a name="Types"></a>Types 

From the built-in scalars and the enums you add, you can generate types in the usual way for GraphQL.  For example:

```graphql
enum Tag {
    GraphQL
    Database
    Dgraph
}

type Post {
    id: ID!
    title: String!
    text: String
    datePublished: DateTime
    tags: [Tag!]!
    author: Author!
}

type Author {
    id: ID!
    name: String!
    posts: [Post!]
    friends: [Author]
}
```

* *Schema rule*: Lists of lists aren't accepted.  For example: `multiTags: [[Tag!]]` isn't valid.
* *Schema rule*: Fields with arguments are not accepted in the input schema.

# <a name="Interfaces"></a>Interfaces

GraphQL interfaces allow you to define a generic pattern that multiple types follow.  When a type implements an interface, that means it has all fields of the interface and some extras.  

When a type implements an interface, GraphQL requires that the type repeats all the fields from the interface, but that's just boilerplate and a maintenance problem, so Dgraph doesn't need that repetition in the input schema and will generate the correct GraphQL for you.

For example, the following defines the schema for posts with comment threads; Dgraph will fill in the `Question` and `Comment` types to make the full GraphQL types.

```graphql
interface Post {
    id: ID!
    text: String
    datePublished: DateTime
}

type Question implements Post {
    title: String!
}

type Comment implements Post {
    commentsOn: Post!
}
```

The generated GraphQL will contain the full types, for example, `Question` gets expanded as:

```graphql
type Question implements Post {
    id: ID!
    text: String
    datePublished: DateTime
    title: String!
}
```

while `Comment` gets expanded as:

```graphql
type Comment implements Post {
    id: ID!
    text: String
    datePublished: DateTime
    commentsOn: Post!
}
```

# <a name="Directives"></a>Directives

Dgraph uses the types and fields in the schema to work out what to accept for mutations and what shape responses should take.  Dgraph also defines a set of GraphQL directives that it uses to further refine what services the GraphQL API offers.  In particular, how to handle two-way edges and what search capability to build in.

## <a name="Inverse"></a>Inverse Edges

GraphQL schemas are always under-specified in that

```graphql
type Author {
    ...
    posts: [Post]
}

type Post {
    ...
    author: Author
}
```

says that an author has a list of posts and a post has an author, but it doesn't tell us that every post in the list of posts for an author has that author as their `author`.  In GraphQL, it's left up to the implementation to make the two-way connection.  Here, we'd expect an author to be the author of all their posts, but that's not what GraphQL enforces.

There's not always a two-way edge. Consider if `Author` were defined as:

```graphql
type Author {
    ...
    posts: [Post]
    liked: [Post]
}
```

There should be no two-way edge for `liked`.  

In Dgraph, the directive `@hasInverse` is used to sort out which edges are bi-directional and which aren't. Adding

```graphql
type Author {
    ...
    posts: [Post] @hasInverse(field: author)
    liked: [Post]
}

type Post {
    ...
    author: Author @hasInverse(field: posts)
}
```

tells Dgraph to link `posts` and `author`.  When a new post is added Dgraph ensures that it's also in the list of its author's posts.  Field   `liked`, on the other hand, has no such linking.

## <a name="Search"></a>Search

The `@search` directive tells Dgraph what search to build into your API.

When a type contains an `@search` directive, Dgraph constructs a search input type and a query in the GraphQL `Query` type. For example, if the schema contains

```graphql
type post {
    ...
    text: Int @search(by: [term])
}
```

then, Dgraph constructs an input type `PostFilter` and adds all possible search options for posts to that.  The search options it constructs are different for each type and argument to `@search` as explained below.

### Int, Float and DateTime

Search for fields of types `Int`, `Float` and `DateTime` is enabled by adding `@search` to the field.  For example, if a schema contains:

```graphql
type Post {
    ...
    numLikes: Int @search
}
```

Dgraph generates search into the API for `numLikes` in two ways: a query for posts and field search on any post list.

A field `queryPost` is added to the `Query` type of the schema.

```graphql
queryPost(filter: PostFilter, order: PostOrder, first: Int, offset: Int): [Post]
```

`PostFilter` will contain less than `lt`, less than or equal to `le`, equal `eq`, greater than or equal to `ge` and greater than `gt` search on `numLikes`.  Allowing for example:

```graphql
queryPost(filter: { numLikes: { gt: 50 }}) { ... }
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

That allows search within the GraphQL query.  For example, to find Karthic's posts with more than 50 likes.

```graphql
queryAuthor(filter: { name: { eq: "Karthic" } } ) {
    ...
    posts(filter: { numLikes: { gt: 50 }}) {
        title
        text
    }
}
```

`DateTime` also allows specifying how the search index should be built: by year, month, day or hour.  `@search` defaults to year, but once you understand your data and query patterns, you might want to changes that like `@search(by: [day])`.

### Boolean

Booleans can only be tested for true or false.  If `isActiveMember: Boolean @search` is in the schema, then the search allows

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

Exact and hash search have the standard lexicographic meaning. Search by regular expression requires bracketing the expression with `/` and `/`.  For example, query for "Karthic" and anyone else with "rti" in their name:

```
queryAuthor(filter: { name: { regexp: "/.*rti.*/" } }) { ... }
```

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
queryPost(filter: { title: { `allofterms: "GraphQL tutorial"` } } ) { ... }
```

will match all posts with both "GraphQL and "tutorial" in the title, while `anyofterms: "GraphQL tutorial"` would match posts with either "GraphQL" or "tutorial".

`fulltext` search is Google-stye text search with stop words, stemming. etc.  So `alloftext: "run woman"` would match "run" as well as "running", etc.  For example, to find posts that talk about fantastic GraphQL tutorials:

```graphql
queryPost(filter: { title: { `alloftext: "fantastic GraphQL tutorials"` } } ) { ... }
```

It's possible to add multiple string indexes to a field.  For example to search for authors by `eq` and regular expressions, add both options to the type definition, as follows.

```graphql
type Author {
    ...
    name: String! @search(by: [hash, regexp])
}
```

### Enums 

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
queryPost(filter: { tags: { eq: GraphQL } } ) { ... }
```

Which would find any post with the `GraphQL` tag.

While `@search(by: [exact, regexp]` would also admit `lt` etc. and 

```graphql
queryPost(filter: { tags: { regexp: "/.*aph.*/" } } ) { ... }
```

which is helpful for example if the enums are something like product codes where regular expressions can match a number of values. 

### and, or, and not

Every search filter contains `and`, `or` and `not`.

GraphQL's syntax is used to write these infix style, so "a and b" is written `a, and: { b }`, and "a or b or c" is written `a, or: { b, or: c }`.  Not is written prefix.

The posts that do not have "GraphQL" in the title.

```graphql
queryPost(filter: { not: { title: { allofterms: "GraphQL"} } } ) { ... }
```

The posts that have "GraphQL" or "Dgraph" in the title.

```graphql
queryPost(filter: { 
    title: { allofterms: "GraphQL"},
    or: { title: { allofterms: "Dgraph" } } 
  } ) { ... }
```

The posts that have "GraphQL" and "Dgraph" in the title.

```graphql
queryPost(filter: { 
    title: { allofterms: "GraphQL"},
    and: { title: { allofterms: "Dgraph" } } 
  } ) { ... }
```

The and is implicit for a single filter object.  The above could be written equivalently as:

```graphql
queryPost(filter: { 
    title: { allofterms: "GraphQL"},
    title: { allofterms: "Dgraph" } 
  } ) { ... }
```

The posts that have "GraphQL" in the title, or have the tag "GraphQL" and mention "Dgraph" in the title

```graphql
queryPost(filter: { 
    title: { allofterms: "GraphQL"},
    or: { title: { allofterms: "Dgraph" }, tags: { eg: "GraphQL" } }
  } ) { ... }
```

### Order and Pagination

Every type with fields whose types can be ordered (`Int`, `Float`, `String`, `DateTime`) gets ordering built into the query and any list fields of that type.  Every query and list field gets pagination with `first` and `after`.

For example, find the most recent 5 posts.

```graphql
queryPost(order: { desc: datePublished }, first: 5) { ... }
```

It's also possible to give multiple orders.  For example, sort by date and within each date order the posts by number of likes.

```graphql
queryPost(order: { desc: datePublished, then: { desc: numLikes } }, first: 5) { ... }
```

# <a name="schemafragment"></a>Dgraph Schema Fragment

While editing your schema, you might find it useful to include this GraphQL schema fragment.  It sets up the definitions of the directives, etc. (like `@search`) that you'll use in your schema.  If your editor is GraphQL aware, it will give you errors if you don't have this available.

Don't include it in your input schema to Dgraph - use your editing environment to set it up as an import.  The details will depend on your setup.

```graphql
scalar DateTime

directive @hasInverse(field: String!) on FIELD_DEFINITION
directive @search(by: [DgraphIndex!]) on FIELD_DEFINITION

enum DgraphIndex {
  int
  float
  bool
  hash
  exact
  term
  fulltext
  trigram
  regexp
  year
  month
  day
  hour
}
```

# Reserved Names

Names `Int`, `Float`, `Boolean`, `String`, `DateTime` and `ID` are reserved and cannot be used to define any other identifiers.

For each type, Dgraph generates a number of GraphQL types needed to operate the GraphQL API, these generated type names also can't be present in the input schema.  For example, for a type `Author`, Dgraph generates `AuthorFilter`, `AuthorOrderable`, `AuthorOrder`, `AuthorRef`, `AddAuthorInput`, `UpdateAuthorInput`, `AuthorPatch`, `AddAuthorPayload`, `DeleteAuthorPayload` and `UpdateAuthorPayload`.  Thus if `Author` is present in the input schema, all of those become reserved type names.

# GraphQL Error Propagation

Before returning query and mutation results, Dgraph uses the types in the schema to apply GraphQL [value completion](https://graphql.github.io/graphql-spec/June2018/#sec-Value-Completion) and [error handling](https://graphql.github.io/graphql-spec/June2018/#sec-Errors-and-Non-Nullability).  That is, `null` values for non-nullable fields, e.g. `String!`, cause error propagation to parent fields.  

In short, the GraphQL value completion and error propagation mean the following.

* Fields marked as nullable (i.e. without `!`) can return `null` in the json response.
* For fields marked as non-nullable (i.e. with `!`) Dgraph never returns null for that field.
* If an instance of type has a non-nullable field that has evaluated to null, the whole instance results in null.
* Reducing an object to null might cause further error propagation.  For example, querying for a post that has an author with a null name results in null: the null name (`name: String!`) causes the author to result in null, and a null author causes the post (`author: Author!`) to result in null.
* Error propagation for lists with nullable elements, e.g. `friends [Author]`, can result in nulls inside the result list.
* Error propagation for lists with non-nullable elements results in null for `friends [Author!]` and would cause further error propagation for `friends [Author!]!`.

Note that, a query that results in no values for a list will always return the empty list `[]`, not `null`, regardless of the nullability.  For example, given a schema for an author with `posts: [Post!]!`, if an author has not posted anything and we queried for that author, the result for the posts field would be `posts: []`.  

A list can, however, result in null due to GraphQL error propagation.  For example, if the definition is `posts: [Post!]`, and we queried for an author who has a list of posts.  If one of those posts happened to have a null title (title is non-nullable `title: String!`), then that post would evaluate to null, the `posts` list can't contain nulls and so the list reduces to null.

# What's to come for Dgraph GraphQL

Dgraph's GraphQL features are in active development.  On the way soon are more search features as well as better mutations.  Also, expect to see GraphQL subscriptions and authorization and authentication features built in.  For existing Dgraph users, we'll be adding features to boot your existing schema into GraphQL and ways you can define your own queries backed by whatever GraphQL+- you like.

## Admin

The admin API and how to run Dgraph with GraphQL.

## Running

The simplest way to start with Dgraph GraphQL is to run the all-in-one Docker image.

```sh
docker run -it -p 8080:8080 dgraph/standalone:v20.03.1
```

That brings up GraphQL at `localhost:8080/graphql` and `localhost:8080/admin`, but is intended for quickstart and doesn't persist data.

## Advanced Options

Once you've tried out Dgraph GraphQL, you'll need to move past the `dgraph/standalone` and run and deploy Dgraph instances.

Dgraph is a distributed graph database.  It can scale to huge data and shard that data across a cluster of Dgraph instances.  GraphQL is built into Dgraph in its Alpha nodes. To learn how to manage and deploy a Dgraph cluster to build an App check Dgraph's [Dgraph docs](https://docs.dgraph.io/), and, in particular, the [deployment guide](https://docs.dgraph.io/deploy/).

GraphQL schema introspection is enabled by default, but can be disabled with the `--graphql_introspection=false` when starting the Dgraph alpha nodes.

## Dgraph's schema

Dgraph's GraphQL runs in Dgraph and presents a GraphQL schema where the queries and mutations are executed in the Dgraph cluster.  So the GraphQL schema is backed by Dgraph's schema.

**Warning: this means if you have a Dgraph instance and change its GraphQL schema, the schema of the underlying Dgraph will also be changed!**

# /admin

When you start Dgraph with GraphQL, two GraphQL endpoints are served.

At `/graphql` you'll find the GraphQL API for the types you've added.  That's what your app would access and is the GraphQL entry point to Dgraph.  If you need to know more about this, see the quick start, example and schema docs.

At `/admin` you'll find an admin API for administering your GraphQL instance.  The admin API is a GraphQL API that serves POST and GET as well as compressed data, much like the `/graphql` endpoint.

Here are the important types, queries, and mutations from the admin schema.

```graphql
type GQLSchema {
	id: ID!
	schema: String! 
	generatedSchema: String!
}

type UpdateGQLSchemaPayload {
	gqlSchema: GQLSchema
}

input UpdateGQLSchemaInput {
	set: GQLSchemaPatch!
}

input GQLSchemaPatch {
	schema: String!
}

type Query {
	getGQLSchema: GQLSchema
	health: Health
}

type Mutation {
	updateGQLSchema(input: UpdateGQLSchemaInput!) : UpdateGQLSchemaPayload
}
```

You'll notice that the /admin schema is very much the same as the schemas generated by Dgraph GraphQL.

* The `health` query lets you know if everything is connected and if there's a schema currently being served at `/graphql`.
* The `getGQLSchema` query gets the current GraphQL schema served at `/graphql`, or returns null if there's no such schema.
* The `updateGQLSchema` mutation allows you to change the schema currently served at `/graphql`.

## First Start

On first starting with a blank database:

* There's no schema served at `/graphql`.
* Querying the `/admin` endpoint for `getGQLSchema` returns `"getGQLSchema": null`.
* Querying the `/admin` endpoint for `health` lets you know that no schema has been added.

## Adding Schema

Given a blank database, running the `/admin` mutation:

```graphql
mutation {
  updateGQLSchema(
    input: { set: { schema: "type Person { name: String }"}})
  {
    gqlSchema {
      schema
      generatedSchema
    }
  }
}
```

would cause the following.

* The `/graphql` endpoint would refresh and now serves the GraphQL schema generated from type `type Person { name: String }`: that's Dgraph type `Person` and predicate `Person.name: string .`; see [here](/dgraph) for how to customize the generated schema.
* The schema of the underlying Dgraph instance would be altered to allow for the new `Person` type and `name` predicate.
* The `/admin` endpoint for `health` would return that a schema is being served.
* The mutation returns `"schema": "type Person { name: String }"` and the generated GraphQL schema for `generatedSchema` (this is the schema served at `/graphql`).
* Querying the `/admin` endpoint for `getGQLSchema` would return the new schema.

## Migrating Schema

Given an instance serving the schema from the previous section, running an `updateGQLSchema` mutation with the following input

```graphql
type Person {
    name: String @search(by: [regexp])
    dob: DateTime
}
```

changes the GraphQL definition of `Person` and results in the following.

* The `/graphql` endpoint would refresh and now serves the GraphQL schema generated from the new type.
* The schema of the underlying Dgraph instance would be altered to allow for `dob` (predicate `Person.dob: datetime .` is added, and `Person.name` becomes `Person.name: string @index(regexp).`) and indexes are rebuilt to allow the regexp search.
* The `health` is unchanged.
* Querying the `/admin` endpoint for `getGQLSchema` now returns the updated schema.

## Removing from Schema

Adding a schema through GraphQL doesn't remove existing data (it would remove indexes).  For example, starting from the schema in the previous section and running `updateGQLSchema` with the initial `type Person { name: String }` would have the following effects.

* The `/graphql` endpoint would refresh to serve the schema built from this type.
* Thus field `dob` would no longer be accessible and there'd be no search available on `name`.
* The search index on `name` in Dgraph would be removed.
* The predicate `dob` in Dgraph is left untouched - the predicate remains and no data is deleted.

## GraphQL on Existing Dgraph

How to use GraphQL on an existing Dgraph instance.

If you have an existing Dgraph instance and want to also expose GraphQL, you need to add a GraphQL schema that maps to your Dgraph schema.  You don't need to expose your entire Dgraph schema as GraphQL, but do note that adding a GraphQL schema can alter the Dgraph schema.

Dgraph also allows type and edge names that aren't valid in GraphQL, so, often, you'll need to expose valid GraphQL names. Dgraph admits special characters and even different languages (see [here](https://docs.dgraph.io/query-language/#predicate-name-rules)), while the GraphQL Spec requires that type and field (predicate) names are generated from `/[_A-Za-z][_0-9A-Za-z]*/`.

# Mapping GraphQL to a Dgraph schema

By default, Dgraph generates a new predicate for each field in a GraphQL type. The name of the generated predicate is composed of the type name followed by a dot `.` and ending with the field name. Therefore, two different types with fields of the same name will turn out to be different Dgraph predicates and can have different indexes.  For example, the types:

```graphql
type Person {
    name: String @search(by: [hash])
    age: Int
}

type Movie {
    name: String @search(by: [term])
}
```

generate a Dgraph schema like:

```graphql
type Person {
    Person.name
    Person.age
}

type Movie {
    Movie.name
}

Person.name: string @index(hash) .
Person.age: int .
Movie.name: string @index(term) .
```

This behavior can be customized with the `@dgraph` directive.  

* `type T @dgraph(type: "DgraphType")` controls what Dgraph type is used for a GraphQL type.
* `field: SomeType @dgraph(pred: "DgraphPredicate")` controls what Dgraph predicate is mapped to a GraphQL field.

For example, if you have existing types that don't match GraphQL requirements, you can create a schema like the following.

```graphql
type Person @dgraph(type: "Human-Person") {
    name: String @search(by: [hash]) @dgraph(pred: "name")
    age: Int
}

type Movie @dgraph(type: "film") {
    name: String @search(by: [term]) @dgraph(pred: "film.name")
}
```

Which maps to the Dgraph schema:

```graphql
type Human-Person {
    name
    Person.age
}

type film {
    film.name
}

name string @index(hash) .
Person.age: int .
film.name string @index(term) .
```

You might also have the situation where you have used `name` for both movie names and people's names.  In this case you can map fields in two different GraphQL types to the one Dgraph predicate.

```graphql
type Person {
    name: String @dgraph(pred: "name")
    ...
}

type Movie {
    name: String @dgraph(pred: "name")
    ...
}
```

*Note: the current behavior requires that when two fields are mapped to the same Dgraph predicate both should have the same `@search` directive.  This is likely to change in a future release where the underlying Dgraph indexes will be the union of the `@search` directives, while the generated GraphQL API will expose only the search given for the particular field.  Allowing, for example, dgraph predicate name to have `term` and `hash` indexes, but exposing only term search for GraphQL movies and hash search for GraphQL people.*

# Roadmap

Be careful with mapping to an existing Dgraph instance.  Updating the GraphQL schema updates the underlying Dgraph schema. We understand that exposing a GraphQL API on an existing Dgraph instance is a delicate process and we plan on adding multiple checks to ensure the validity of schema changes to avoid issues caused by detectable mistakes.

Future features are likely to include:

* Generating a first pass GraphQL schema from an existing dgraph schema.
* A way to show what schema diff will happen when you apply a new GraphQL schema.
* Better handling of `@dgraph` with `@search`

We look forward to you letting us know what features you'd like, so please join us on [discuss](https://discuss.dgraph.io/), [slack](https://slack.dgraph.io/) or [GitHub](https://github.com/dgraph-io/dgraph).
