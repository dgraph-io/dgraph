+++
title = "Get Started with Dgraph - String Indices and Modeling Tweet Graph"
+++

**Welcome to the fifth tutorial of getting started with Dgraph.**

In the [previous tutorial]({{< relref "tutorial-4/index.md" >}}), we learned about using multi-language strings and operations on them using [language tags](https://www.w3schools.com/tags/ref_language_codes.asp).

In this tutorial, we'll model tweets in Dgraph and, using it, we'll learn more about string indices in Dgraph.

We'll specifically learn about:

- Modeling tweets in Dgraph.
- Using String indices in Dgraph
  - Querying twitter users using the `hash` index.
  - Comparing strings using the `exact` index.
  - Searching for tweets based on keywords using the `term` index.

Here's the complimentary video for this blog post. It'll walk you through the steps of this getting started episode.

{{< youtube Ww5cwixwkHo >}}

Let's start analyzing the anatomy of a real tweet and figure out how to model it in Dgraph.

The accompanying video of the tutorial will be out shortly, so stay tuned to [our YouTube channel](https://www.youtube.com/channel/UCghE41LR8nkKFlR3IFTRO4w).

## Modeling a tweet in Dgraph

Here's a sample tweet.

{{< tweet 1194740206177402880>}}

Let's dissect the tweet above. Here are the components of the tweet:

- **The Author**

  The author of the tweet is the user `@hackintoshrao`.

- **The Body**

  This component is the content of the tweet.

  > Test tweet for the fifth episode of getting started series with @dgraphlabs.
Wait for the video of the fourth one by @francesc the coming Wednesday!
#GraphDB #GraphQL

- **The Hashtags**

  Here are the hashtags in the tweet: `#GraphQL` and `#GraphDB`.

- **The Mentions**

  A tweet can mention other twitter users.

  Here are the mentions in the tweet above: `@dgraphlabs` and `@francesc`.

Before we model tweets in Dgraph using these components, let's recap the design principles of a graph model:

> `Nodes` and `Edges` are the building blocks of a graph model.
May it be a sale, a tweet, user info, any concept or an entity is represented as a node.
If any two nodes are related, represent that by creating an edge between them.

With the above design principles in mind, let's go through components of a tweet and see how we could fit them into Dgraph.

**The Author**

The Author of a tweet is a twitter user. We should use a node to represent this.

**The Body**

We should represent every tweet as a node.

**The Hashtags**

It is advantageous to represent a hashtag as a node of its own.
It gives us better flexibility while querying.

Though you can search for hashtags from the body of a tweet, it's not efficient to do so.
Creating unique nodes to represent a hashtag, allows you to write performant queries like the following: _Hey Dgraph, give me all the tweets with hashtag #graphql_

**The Mentions**

A mention represents a twitter user, and we've already modeled a user as a node.
Therefore, we represent a mention as an edge between a tweet and the users mentioned.

### The Relationships

We have three types of nodes: `User`, `Tweet,` and `Hashtag`.

{{% load-img "/images/tutorials/5/a-nodes.jpg" "graph nodes" %}}

Let's look at how these nodes might be related to each other and model their relationship as an edge between them.

**The User and Tweet nodes**

There's a two-way relationship between a `Tweet` and a `User` node.

- Every tweet is authored by a user, and a user can author many tweets.

Let's name the edge representing this relationship  as `authored` .

An `authored` edge points from a `User` node to a `Tweet` node.

- A tweet can mention many users, and users can be mentioned in many tweets.

Let's name the edge which represents this relationship as `mentioned`.

A `mentioned` edge points from a `Tweet` node to a `User` node.
These users are the ones who are mentioned in the tweet.

{{% load-img "/images/tutorials/5/a-tweet-user.jpg" "graph nodes" %}}

**The tweet and the hashtag nodes**

A tweet can have one or more hashtags.
Let's name the edge, which represents this relationship as `tagged_with`.


A `tagged_with` edge points from a `Tweet` node to a `Hashtag` node.
These hashtag nodes correspond to the hashtags in the tweets.

{{% load-img "/images/tutorials/5/a-tagged.jpg" "graph nodes" %}}

**The Author and hashtag nodes**

There's no direct relationship between an author and a hashtag node.
Hence, we don't need a direct edge between them.

Our graph model of a tweet is ready! Here's it is.

{{% load-img "/images/tutorials/5/a-graph-model.jpg" "tweet model" %}}

Here is the graph of our sample tweet.

{{% load-img "/images/tutorials/5/c-tweet-model.jpg" "tweet model" %}}

Let's add a couple of tweets to the list.

{{< tweet 1142124111650443273>}}

{{< tweet 1192822660679577602>}}

We'll be using these two tweets and the sample tweet, which we used in the beginning as our dataset.
Open Ratel, go to the mutate tab, paste the mutation, and click Run.

```json
{
  "set": [
    {
      "user_handle": "hackintoshrao",
      "user_name": "Karthic Rao",
      "uid": "_:hackintoshrao",
      "authored": [
        {
          "tweet": "Test tweet for the fifth episode of getting started series with @dgraphlabs. Wait for the video of the fourth one by @francesc the coming Wednesday!\n#GraphDB #GraphQL",
          "tagged_with": [
            {
              "uid": "_:graphql",
              "hashtag": "GraphQL"
            },
            {
              "uid": "_:graphdb",
              "hashtag": "GraphDB"
            }
          ],
          "mentioned": [
            {
              "uid": "_:francesc"
            },
            {
              "uid": "_:dgraphlabs"
            }
          ]
        }
      ]
    },
    {
      "user_handle": "francesc",
      "user_name": "Francesc Campoy",
      "uid": "_:francesc",
      "authored": [
        {
          "tweet": "So many good talks at #graphqlconf, next year I'll make sure to be *at least* in the audience!\nAlso huge thanks to the live tweeting by @dgraphlabs for alleviating the FOMO😊\n#GraphDB ♥️ #GraphQL",
          "tagged_with": [
            {
              "uid": "_:graphql"
            },
            {
              "uid": "_:graphdb"
            },
            {
              "hashtag": "graphqlconf"
            }
          ],
          "mentioned": [
            {
              "uid": "_:dgraphlabs"
            }
          ]
        }
      ]
    },
    {
      "user_handle": "dgraphlabs",
      "user_name": "Dgraph Labs",
      "uid": "_:dgraphlabs",
      "authored": [
        {
          "tweet": "Let's Go and catch @francesc at @Gopherpalooza today, as he scans into Go source code by building its Graph in Dgraph!\nBe there, as he Goes through analyzing Go source code, using a Go program, that stores data in the GraphDB built in Go!\n#golang #GraphDB #Databases #Dgraph ",
          "tagged_with": [
            {
              "hashtag": "golang"
            },
            {
              "uid": "_:graphdb"
            },
            {
              "hashtag": "Databases"
            },
            {
              "hashtag": "Dgraph"
            }
          ],
          "mentioned": [
            {
              "uid": "_:francesc"
            },
            {
              "uid": "_:dgraphlabs"
            }
          ]
        },
        {
          "uid": "_:gopherpalooza",
          "user_handle": "gopherpalooza",
          "user_name": "Gopherpalooza"
        }
      ]
    }
  ]
}
```

_Note: If you're new to Dgraph, and yet to figure out how to run the database and use Ratel, we highly recommend reading the [first article of the series]({{< relref "tutorial-1/index.md" >}})_

Here is the graph we built.

{{% load-img "/images/tutorials/5/x-all-tweets.png" "tweet graph" %}}

Our graph has:

- Five blue twitter user nodes.
- The green nodes are the tweets.
- The blue ones are the hashtags.

Let's start our tweet exploration by querying for the twitter users in the database.

```
{
  tweet_graph(func: has(user_handle)) {
     user_handle
  }
}
```

{{% load-img "/images/tutorials/5/j-users.png" "tweet model" %}}

_Note: If the query syntax above looks not so familiar to you, check out the [first tutorial]({{< relref "tutorial-1/index.md" >}})._

We have four twitter users: `@hackintoshrao`, `@francesc`, `@dgraphlabs`, and `@gopherpalooza`.

Now, let's find their tweets and hashtags too.

```graphql
{
  tweet_graph(func: has(user_handle)) {
     user_name
     authored {
      tweet
      tagged_with {
        hashtag
      }
    }
  }
}
```

{{% load-img "/images/tutorials/5/y-author-tweet.png" "tweet model" %}}

_Note: If the traversal query syntax in the above query is not familiar to you, [check out the third tutorial]({{< relref "tutorial-3/index.md" >}}) of the series._

Before we start querying our graph, let's learn a bit about database indices using a simple analogy.

### What are indices?

Indexing is a way to optimize the performance of a database by minimizing the number of disk accesses required when a query is processed.

Consider a "Book" of 600 pages, divided into 30 sections.
Let's say each section has a different number of pages in it.

Now, without an index page, to find a particular section that starts with the letter "F", you have no other option than scanning through the entire book. i.e: 600 pages.

But with an index page at the beginning makes it easier to access the intended information.
You just need to look over the index page, after finding the matching index, you can efficiently jump to the section by skipping other sections.

But remember that the index page also takes disk space!
Use them only when necessary.

In our next section,let's learn some interesting queries on our twitter graph.

## String indices and querying

### Hash index

Let's compose a query which says: _Hey Dgraph, find me the tweets of user with twitter handle equals to `hackintoshrao`._

Before we do so, we need first to add an index has to the `user_handle` predicate.
We know that there are 5 types of string indices: `hash`, `exact`, `term`, `full-text`, and `trigram`.

The type of string index to be used depends on the kind of queries you want to run on the string predicate.

In this case, we want to search for a node based on the exact string value of a predicate.
For a use case like this one, the `hash` index is recommended.

Let's first add the `hash` index to the `user_handle` predicate.

{{% load-img "/images/tutorials/5/k-hash.png" "tweet model" %}}

Now, let's use the `eq` comparator to find all the tweets of `hackintoshrao`.

Go to the query tab, type in the query, and click Run.

```graphql
 {
  tweet_graph(func: eq(user_handle, "hackintoshrao")) {
     user_name
     authored {
		tweet
    }
  }
}
```

{{% load-img "/images/tutorials/5/z-exact.png" "tweet model" %}}

_Note: Refer to [the third tutorial]({{< relref "tutorial-3/index.md" >}}), if you want to know about comparator functions like `eq` in detail._

Let's extend the last query also to fetch the hashtags and the mentions.

```graphql
{
  tweet_graph(func: eq(user_handle, "hackintoshrao")) {
     user_name
     authored {
      tweet
      tagged_with {
        hashtag
      }
      mentioned {
        user_name
      }
    }
  }
}
```

{{% load-img "/images/tutorials/5/l-hash-query.png" "tweet model" %}}

_Note: If the traversal query syntax in the above query is not familiar to you, [check out the third tutorial]({{< relref "tutorial-3/index.md" >}}) of the series._

Did you know that string values in Dgraph can also be compared using comparators like greater-than or less-than?

In our next section, let's see how to run the comparison functions other than `equals to (eq)` on the string predicates.

### Exact Index

We discussed in the [third tutorial]({{< relref "tutorial-3/index.md" >}}) that there five comparator functions in Dgraph.

Here's a quick recap:

| comparator function name | Full form |
|--------------------------|--------------------------|
| eq | equals to |
| lt | less than |
| le | less than or equal to |
| gt | greater than |
| ge | greater than or equal to |

All five comparator functions can be applied to the string predicates.

We have already used the `eq` operator.
The other four are useful for operations, which depend on the alphabetical ordering of the strings.

Let's learn about it with a simple example.

Let's find the twitter accounts which come after `dgraphlabs` in alphabetically sorted order.

```graphql
{
  using_greater_than(func: gt(user_handle, "dgraphlabs")) {
    user_handle
  }
}
```

{{% load-img "/images/tutorials/5/n-exact-error.png" "tweet model" %}}

Oops, we have an error!

You can see from the error that the current `hash` index on the `user_handle` predicate doesn't support the `gt` function.

To be able to do string comparison operations like the one above, you need first set the `exact` index on the string predicate.

The `exact` index is the only string index that allows you to use the `ge`, `gt`, `le`, `lt` comparators on the string predicates.

Remind you that the `exact` index also allows you to use `equals to (eq)` comparator.
But, if you want to just use the `equals to (eq)` comparator on string predicates, using the `exact` index would be an overkill.
The `hash` index would be a better option, as it is, in general, much more space-efficient.

Let's see the `exact` index in action.

{{% load-img "/images/tutorials/5/o-exact-conflict.png" "set exact" %}}

We again have an error!

Though a string predicate can have more than one index, some of them are not compatible with each other.
One such example is the combination of the `hash` and the `exact` indices.

The `user_handle` predicate already has the `hash` index, so trying to set the `exact` index gives you an error.

Let's uncheck the `hash` index for the `user_handle` predicate, select the `exact` index, and click update.

{{% load-img "/images/tutorials/5/p-set-exact.png" "set exact" %}}

Though Dgraph allows you to change the index type of a predicate, do it only if it's necessary.
When the indices are changed, the data needs to be re-indexed, and this takes some computing, so it could take a bit of time.
While the re-indexing operation is running, all mutations will be put on hold.

Now, let's re-run the query.

{{% load-img "/images/tutorials/5/q-exact-gt.png" "tweet model" %}}

The result contains three twitter handles: `francesc`, `gopherpalooza`, and `hackintoshrao`.

In the alphabetically sorted order, these twitter handles are greater than `dgraphlabs`.

Some tweets appeal to us better than others.
For instance, I love `Graphs` and `Go`.
Hence, I would surely enjoy tweets that are related to these topics.
A keyword-based search is a useful way to find relevant information.

Can we search for tweets based on one or more keywords related to your interests?

Yes, we can! Let's do that in our next section.

### The Term index

The `term` index lets you search string predicates based on one or more keywords.
These keywords are called terms.

To be able to search tweets with specific keywords or terms, we need to first set the `term` index on the tweets.

Adding the `term` index is similar to adding any other string index.

{{% load-img "/images/tutorials/5/r-term-set.png" "term set" %}}

Dgraph provides two built-in functions specifically to search for terms: `allofterms` and `anyofterms`.

Apart from these two functions, the `term` index only supports the `eq` comparator.
This means any other query functions (like lt, gt, le...) fails when run on string predicates with the `term` index.

We'll soon take a look at the table containing the string indices and their supporting query functions.
But first, let's learn how to use `anyofterms` and `allofterms` query functions.
Let's write a query to find all tweets with terms or keywords `Go` or `Graph` in them.

Go the query tab, paste the query, and click Run.

```graphql
{
  find_tweets(func: anyofterms(tweet, "Go Graph")) {
    tweet
  }
}
```

Here's the matched tweet from the query response:

```json
{
        "tweet": "Let's Go and catch @francesc at @Gopherpalooza today, as he scans into Go source code by building its Graph in Dgraph!\nBe there, as he Goes through analyzing Go source code, using a Go program, that stores data in the GraphDB built in Go!\n#golang #GraphDB #Databases #Dgraph "
}
```

{{% load-img "/images/tutorials/5/s-go-graph.png" "go graph set" %}}

_Note: Check out [the first tutorial]({{< relref "tutorial-1/index.md" >}}) if the query syntax, in general, is not familiar to you_

The `anyofterms` function returns tweets which have either of `Go` or `Graph` keyword.

In this case, we've used only two terms to search for (`Go` and `Graph`), but you can extend for any number of terms to be searched or matched.

The result has one of the three tweets in the database.
The other two tweets don't make it to the result since they don't have either of the terms `Go` or `Graph`.

It's also important to notice that the term search functions (`anyofterms` and `allofterms`) are insensitive to case and special characters.

This means, if you search for the term `GraphQL`, the query returns a positive match for all of the following terms found in the tweets: `graphql`, `graphQL`, `#graphql`, `#GraphQL`.

Now, let's find tweets that have either of the terms `Go` or `GraphQL` in them.


```graphql
{
  find_tweets(func: anyofterms(tweet, "Go GraphQL")) {
    tweet
  }
}
```

{{% load-img "/images/tutorials/5/t-go-graphql-all.png" "Go Graphql" %}}

Oh wow, we have all the three tweets in the result.
This means, all of the three tweets have either of the terms `Go` or `GraphQL`.

Now, how about finding tweets that contain both the terms `Go` and `GraphQL` in them.
We can do it by using the `allofterms` function.

```graphql
{
  find_tweets(func: allofterms(tweet, "Go GraphQL")) {
    tweet
  }
}
```

{{% load-img "/images/tutorials/5/u-allofterms.png" "Go Graphql" %}}

We have an empty result.
None of the tweets have both the terms `Go` and `GraphQL` in them.

Besides `Go` and `Graph`, I'm also a big fan of `GraphQL` and `GraphDB`.

Let's find out tweets that contain both the keywords `GraphQL` and `GraphDB` in them.

{{% load-img "/images/tutorials/5/v-graphdb-graphql.png" "Graphdb-GraphQL" %}}

We have two tweets in a result which has both the terms `GraphQL` and `GraphDB`.

```
{
  "tweet": "Test tweet for the fifth episode of getting started series with @dgraphlabs. Wait for the video of the fourth one by @francesc the coming Wednesday!\n#GraphDB #GraphQL"
},
{
  "tweet": "So many good talks at #graphqlconf, next year I'll make sure to be *at least* in the audience!\nAlso huge thanks to the live tweeting by @dgraphlabs for alleviating the FOMO😊\n#GraphDB ♥️ #GraphQL"
}
```

Before we wrap up, here's the table containing the three string indices we learned about, and their compatible built-in functions.

| Index | Valid query functions      |
|-------|----------------------------|
| hash  | eq                         |
| exact | eq, lt, gt, le, ge         |
| term  | eq, allofterms, anyofterms |


## Summary

In this tutorial, we modeled a series of tweets and set up the exact, term, and hash indices in order to query them.

Did you know that Dgraph also offers more powerful search capabilities like full-text search and regular expressions based search?

In the next tutorial, we'll explore these features and learn about more powerful ways of searching for your favorite tweets!

Sounds interesting?
Then see you all soon in the next tutorial. Till then, happy Graphing!

Check out our next tutorial of the getting started series [here]({{< relref "tutorial-6/index.md" >}}).

## Need Help

* Please use [discuss.dgraph.io](https://discuss.dgraph.io) for questions, feature requests, bugs, and discussions.

<style>
  /* blockquote styling */
  blockquote {
    font-size: 1;
    font-style: italic;
    margin: 0 3rem 1rem 3rem;
    text-align: justify;
  }
  blockquote p:last-child, blockquote ul:last-child, blockquote ol:last-child {
    margin-bottom: 0;
  }
  blockquote cite {
    font-size: 15px;
    font-size: 0.9375rem;
    line-height: 1.5;
    font-style: normal;
    color: #555;
  }
  blockquote footer, blockquote small {
    font-size: 18px;
    font-size: 1.125rem;
    display: block;
    line-height: 1.42857143;
  }
  blockquote footer:before, blockquote small:before {
    content: "\2014 \00A0";
  }
</style>
