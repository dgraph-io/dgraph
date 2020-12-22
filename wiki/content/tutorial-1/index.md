+++
title = "Get Started with Dgraph - Introduction"
+++

**Welcome to getting started with Dgraph.**

[Dgraph](https://dgraph.io) is an open-source, transactional, distributed, native Graph Database. Here is the first tutorial of the get started series on using Dgraph.

In this tutorial, we'll learn how to build the following graph on Dgraph,

{{% load-img "/images/tutorials/1/gs-1.JPG" "The simple graph" %}}

In the process, we'll learn about:

- Running Dgraph using the `dgraph/standalone` docker image.
- Running the following basic operations using Dgraph's UI Ratel,
 - Creating a node.
 - Creating an edge between two nodes.
 - Querying for the nodes.

You can see the accompanying video below.

{{< youtube u73ovhDCPQQ >}}

---

## Running Dgraph

Running the `dgraph/standalone` docker image is the quickest way to get started with Dgraph.
This standalone image is meant for quickstart purposes only.
It is not recommended for production environments.

Ensure that [Docker](https://docs.docker.com/install/) is installed and running on your machine.

Now, it's just a matter of running the following command, and you have Dgraph up and running.

```sh
docker run --rm -it -p 8000:8000 -p 8080:8080 -p 9080:9080 dgraph/standalone:{{< version >}}
```

### Nodes and Edges

In this section, we'll build a simple graph with two nodes and an edge connecting them.

{{% load-img "/images/tutorials/1/gs-1.JPG" "The simple graph" %}}

In a Graph Database, concepts or entities are represented as nodes.
May it be a sale, a transaction, a place, or a person, all these entities are
represented as nodes in a Graph Database.

An edge represents the relationship between two nodes.
The two nodes in the above graph represent people: `Karthic` and `Jessica`.
You can also see that these nodes have two associated properties: `name` and `age`.
These properties of the nodes are called `predicates` in Dgraph.

Karthic follows Jessica. The `follows` edge between them represents their relationship.
The edge connecting two nodes is also called a `predicate` in Dgraph,
although this one points to another node rather than a string or an integer.

The `dgraph/standalone` image setup comes with the useful Dgraph UI called Ratel.
Just visit [http://localhost:8000](http://localhost:8000) from your browser, and you will be able to access it.

{{% load-img "/images/tutorials/1/gs-2.png" "ratel-1" %}}

We'll be using the latest stable release of Ratel.

{{% load-img "/images/tutorials/1/gs-3.png" "ratel-2" %}}

### Mutations using Ratel

The create, update, and delete operations in Dgraph are called mutations.

Ratel makes it easier to run queries and mutations.
We'll be exploring more of its features all along with the tutorial series.

Let's go to the Mutate tab and paste the following mutation into the text area.
_Do not execute it just yet!_

```json
{
  "set": [
    {
      "name": "Karthic",
      "age": 28
    },
    {
      "name": "Jessica",
      "age": 31
    }
  ]
}
```

The query above creates two nodes, one corresponding to each of the JSON values associated with `"set"`.
However, it doesn't create an edge between these nodes.

A small modification to the mutation will fix it, so it creates an edge in between them.

```json
{
  "set": [
    {
      "name": "Karthic",
      "age": 28,
      "follows": {
        "name": "Jessica",
        "age": 31
      }
    }
  ]
}
```

{{% load-img "/images/tutorials/1/explain-query.JPG" "explain mutation" %}}

Let's execute this mutation. Click Run and boom!

{{% load-img "/images/tutorials/1/mutate-example.gif" "Query-gif" %}}

You can see in the response that two UIDs (Universal IDentifiers) have been created.
The two values in the `"uids"` field of the response correspond
to the two nodes created for "Karthic" and "Jessica".

### Querying using the has function

Now, let's run a query to visualize the nodes which we just created.
We'll be using Dgraph's `has` function.
The expression `has(name)` returns all the nodes with a predicate `name` associated with them.

```sh
{
  people(func: has(name)) {
    name
    age
  }
}
```

Go to the `Query` tab this time and type in the query above.
Then, click `Run` on the top right of the screen.

{{% load-img "/images/tutorials/1/query-1.png" "query-1" %}}

Ratel renders a graph visualization of the result.

Just click on any of them, notice that the nodes are assigned UIDs,
matching the ones, we saw in the mutation's response.

You can also view the JSON results in the JSON tab on the right.

{{% load-img "/images/tutorials/1/query-2.png" "query-2" %}}

#### Understanding the query

{{% load-img "/images/tutorials/1/explain-query-2.JPG" "Illustration with explanation" %}}

The first part of the query is the user-defined function name.
In our query, we have named it as `people`. However, you could use any other name.

The `func` parameter has to be associated with a built-in function of Dgraph.
Dgraph offers a variety of built-in functions. The `has` function is one of them.
Check out the [query language guide](https://dgraph.io/docs/query-language) to know more about other built-in functions in Dgraph.

The inner fields of the query are similar to the column names in a SQL select statement or to a GraphQL query!

You can easily specify which predicates you want to get back.

```graphql
{
  people(func: has(name)) {
    name
  }
}
```

Similarly, you can use the `has` function to find all nodes with the `age` predicate.

```graphql
{
  people(func: has(age)) {
    name
  }
}
```

### Flexible schema

Dgraph doesn't enforce a structure or a schema. Instead, you can start entering
your data immediately and add constraints as needed.

Let's look at this mutation.

```json
{
  "set": [
    {
      "name": "Balaji",
      "age": 23,
      "country": "India"
    },
    {
      "name": "Daniel",
      "age": 25,
      "city": "San Diego"
    }
  ]
}
```

We are creating two nodes, while the first node has predicates `name`, `age`, and `country`,
the second one has `name`, `age`, and `city`.

Schemas are not needed initially. Dgraph creates
new predicates as they appear in your mutations.
This flexibility can be beneficial, but if you prefer to force your
mutations to follow a given schema there are options available that
we will explore in an next tutorial.

## Wrapping up

In this tutorial, we learned the basics of Dgraph, including how to
run the database, add new nodes and predicates, and query them
back.

Before we wrap, here are some quick bits about the next tutorial.
Did you know that the nodes can also be fetched given their UID?
They also can be used to create an edge between existing nodes!

Sounds interesting?

Check out our next tutorial of the getting started series [here]({{< relref "tutorial-2/index.md" >}}).

## Need Help

* Please use [discuss.dgraph.io](https://discuss.dgraph.io) for questions, feature requests, bugs, and discussions.
