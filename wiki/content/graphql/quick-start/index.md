+++
title = "Quick Start"
weight = 1
[menu.main]
  url = "/graphql/quick-start/"
  name = "Quick Start"
  identifier = "graphql-quick-start"
  parent = "graphql"
+++

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

It's a one-liner to bring up Dgraph with GraphQL.  *Note: The Dgraph standalone image is great for quick start and exploring, but it's not meant for production use.  Once you want to build an App or persist your data for restarts, you'll need to review the   [admin docs](/graphql/admin).*

```
docker run -it -p 8080:8080 dgraph/standalone:master
```

With that, GraphQL has started at localhost:8080/graphql, but it doesn't have a schema to serve yet. 

## Step 2 - Add a GraphQL Schema

Dgraph will run your GraphQL API at `/graphql` and an admin interface at `/admin`.  The `/admin` interface lets you add and update the GraphQL schema served at `/graphql`.  The quickest way to reset the schema is just to post it to `/admin` with curl.  

Take the schema above, cut-and-paste it into a file called `schema.graphql` and run the following curl command.

```
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