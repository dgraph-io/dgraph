+++
title = "Get Started with Dgraph - Advanced Text Search on Social Graphs"
+++

**Welcome to the sixth tutorial of getting started with Dgraph.**

In the [previous tutorial]({{< relref "tutorial-5/index.md" >}}), we learned about building social graphs in Dgraph, by modeling tweets as an example.
We queried the tweets using the `hash` and `exact` indices, and implemented a keyword-based search to find your favorite tweets using the `term` index and its functions.

In this tutorial, we'll continue from where we left off and learn about advanced text search features in Dgraph.

Specifically, we'll focus on two advanced feature:

- Searching for tweets using Full-text search.
- Searching for hashtags using the regular expression search.

The accompanying video of the tutorial will be out shortly, so stay tuned to [our YouTube channel](https://www.youtube.com/channel/UCghE41LR8nkKFlR3IFTRO4w).

---

Before we dive in, let's do a quick recap of how to model the tweets in Dgraph.

{{% load-img "/images/tutorials/5/a-graph-model.jpg" "tweet model" %}}

In the previous tutorial, we took three real tweets as a sample dataset and stored them in Dgraph using the above graph as a model.

In case you haven't stored the tweets from the [previous tutorial]({{< relref "tutorial-5/index.md" >}}) into Dgraph, here's the sample dataset again.

Copy the mutation below, go to the mutation tab and click Run.

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

_Note: If you're new to Dgraph, and this is the first time you're running a mutation, we highly recommend reading the [first tutorial of the series before proceeding.]({{< relref "tutorial-1/index.md" >}})_

Voilà! Now you have a graph with `tweets`, `users`, and `hashtags`. It is ready for us to explore.

{{% load-img "/images/tutorials/5/x-all-tweets.png" "tweet graph" %}}

_Note: If you're curious to know how we modeled the tweets in Dgraph, refer to [the previous tutorial.]({{< relref "tutorial-5/index.md" >}})_

Let's start by finding your favorite tweets using the full-text search feature first.

## Full text search

Before we learn how to use the Full-text search feature, it's important to understand when to use it.

The length and the number of words in a string predicate value vary based on what the predicates represent.

Some string predicate values have only a few terms (words) in them.
Predicates representing `names`, `hashtags`, `twitter handle`, `city names` are a few good examples. These predicates are easy to query using their exact values.


For instance, here is an example query.

_Give me all the tweets where the user name is equal to `John Campbell`_.

You can easily compose queries like these after adding either the `hash` or an `exact` index to the string predicates.


But, some of the string predicates store sentences. Sometimes even one or more paragraphs of text data in them.
Predicates representing a tweet, a bio, a blog post, a product description, or a movie review are just some examples.
It's relatively hard to query these predicates.

It's not practical to query such predicates using the `hash` or `exact` string indices.
A keyword-based search using the `term` index is a good starting point to query such predicates.
We used it in our [previous tutorial]({{< relref "tutorial-5/index.md" >}}) to find the tweets with an exact match for keywords like `GraphQL`, `Graphs`, and `Go`.

But, for some of the use cases, just the keyword-based search may not be sufficient.
You might need a more powerful search capability, and that's when you should consider using Full-text search.

Let's write some queries and understand Dgraph's Full-text search capability in detail.

To be able to do a Full-text search, you need to first set a `fulltext` index on the `tweet` predicate.

Creating a `fulltext` index on any string predicate is similar to creating any other string indices.

{{% load-img "/images/tutorials/6/a-set-index.png" "full text" %}}

_Note: Refer to the [previous tutorial]({{< relref "tutorial-5/index.md" >}}) if you're not sure about creating an index on a string predicate._

Now, let's do a Full-text search query to find tweets related to the following topic: `graph data and analyzing it in graphdb`.

You can do so by using either of `alloftext` or `anyoftext` in-built functions.
Both functions take two arguments.
The first argument is the predicate to search.
The second argument is the space-separated string values to search for, and we call these as the `search strings`.

```sh
- alloftext(predicate, "space-separated search strings")
- anyoftext(predicate, "space-separated search strings")
```

We'll look at the difference between these two functions later. For now, let's use the `alloftext` function.

Go to the query tab, paste the query below, and click Run.
Here is our search string: `graph data and analyze it in graphdb`.

```graphql
{
  search_tweet(func: alloftext(tweet, "graph data and analyze it in graphdb")) {
    tweet
  }
}
```

{{% load-img "/images/tutorials/6/b-full-text-query-1.png" "tweet graph" %}}

Here's the matched tweet, which made it to the result.

{{< tweet 1192822660679577602>}}

If you observe, you can see some of the words from the search strings are not present in the matched tweet, but the tweet has still made it to the result.

To be able to use the Full-text search capability effectively, we must understand how it works.

Let's understand it in detail.

Once you set a `fulltext` index on the tweets, internally, the tweets are processed, and `fulltext` tokens are generated.
These `fulltext` tokens are then indexed.

The search string also goes through the same processing pipeline, and `fulltext` tokens generated them too.

Here are the steps to generate the `fulltext` tokens:

- Split the tweets into chunks of words called tokens (tokenizing).
- Convert these tokens to lowercase.
- [Unicode-normalize](http://unicode.org/reports/tr15/#Norm_Forms) the tokens.
- Reduce the tokens to their root form, this is called [stemming](https://en.wikipedia.org/wiki/Stemming) (running to run, faster to fast and so on).
- Remove the [stop words](https://en.wikipedia.org/wiki/Stop_words).

You would have seen in [the fourth tutorial]({{< relref "tutorial-4/index.md" >}}) that Dgraph allows you to build multi-lingual apps.

The stemming and stop words removal are not supported for all the languages.
Here is [the link to the docs](https://dgraph.io/docs/query-language/#full-text-search) that contains the list of languages and their support for stemming and stop words removal.

Here is the table with the matched tweet and its search string in the first column.
The second column contains their corresponding `fulltext` tokens generated by Dgraph.

| Actual text data | fulltext tokens generated by Dgraph |
|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| Let's Go and catch @francesc at @Gopherpalooza today, as he scans into Go source code by building its Graph in Dgraph!\nBe there, as he Goes through analyzing Go source code, using a Go program, that stores data in the GraphDB built in Go!\n#golang #GraphDB #Databases #Dgraph | [analyz build built catch code data databas dgraph francesc go goe golang gopherpalooza graph graphdb program scan sourc store todai us] |
| graph data and analyze it in graphdb | [analyz data graph graphdb] |

From the table above, you can see that the tweets are reduced to an array of strings or tokens.

Dgraph internally uses [Bleve package](https://github.com/blevesearch/bleve) to do the stemming.

Here are the `fulltext` tokens generated for our search string: [`analyz`, `data`, `graph`, `graphdb`].

As you can see from the table above, all of the `fulltext` tokens generated for the search string exist in the matched tweet.
Hence, the `alloftext` function returns a positive match for the tweet.
It would not have returned a positive match even if one of the tokens in the search string is missing for the tweet. But, the `anyoftext` function would've returned a positive match as long as the tweets and the search string have at least one of the tokens in common.

If you're interested to see Dgraph's `fulltext` tokenizer in action, [here is the gist](https://gist.github.com/hackintoshrao/0e8d715d8739b12c67a804c7249146a3) containing the instructions to use it.

Dgraph generates the same `fulltext` tokens even if the words in a search string is differently ordered.
Hence, using the same search string with different order would not impact the query result.

As you can see, all three queries below are the same for Dgraph.

```graphql
{
  search_tweet(func: alloftext(tweet, "graph analyze and it in graphdb data")) {
    tweet
  }
}
```

```graphql
{
  search_tweet(func: alloftext(tweet, "data and data analyze it graphdb in")) {
    tweet
  }
}
```

```graphql
{
  search_tweet(func: alloftext(tweet, "analyze data and it in graph graphdb")) {
    tweet
  }
}
```

Now, let's move onto the next advanced text search feature of Dgraph: regular expression based queries.

Let's use them to find all the hashtags containing the following substring: `graph`.

## Regular expression search

[Regular expressions](https://www.geeksforgeeks.org/write-regular-expressions/) are powerful ways of expressing search patterns.
Dgraph allows you to search for string predicates based on regular expressions.
You need to set the `trigram` index on the string predicate to be able to perform regex-based queries.

Using regular expression based search, let's match all the hashtags that have this particular pattern: `Starts and ends with any characters of indefinite length, but with the substring graph in it`.

Here is the regex expression we can use: `^.*graph.*$`

Check out [this tutorial](https://www.geeksforgeeks.org/write-regular-expressions/) if you're not familiar with writing a regular expression.

Let's first find all the hashtags in the database using the `has()` function.

```graphql
{
  hash_tags(func: has(hashtag)) {
    hashtag
  }
}
```

{{% load-img "/images/tutorials/6/has-hashtag.png" "The hashtags" %}}

_If you're not familiar with using the `has()` function, refer to [the first tutorial]({{< relref "tutorial-1/index.md" >}}) of the series._

You can see that we have six hashtags in total, and four of them have the substring `graph` in them: `Dgraph`, `GraphQL`, `graphqlconf`, `graphDB`.

We should use the built-in function `regexp` to be able to use regular expressions to search for predicates.
This function takes two arguments, the first is the name of the predicate, and the second one is the regular expression.

Here is the syntax of the `regexp` function: `regexp(predicate, /regular-expression/)`

Let's execute the following query to find the hashtags that have the substring `graph`.

Go to the query tab, type in the query, and click Run.


```graphql
{
  reg_search(func: regexp(hashtag, /^.*graph.*$/)) {
    hashtag
  }
}
```

Oops! We have an error!
It looks like we forgot to set the `trigram` index on the `hashtag` predicate.

{{% load-img "/images/tutorials/6/trigram-error.png" "The hashtags" %}}

Again, setting a `trigram` index is similar to setting any other string index, let's do that for the `hashtag` predicate.

{{% load-img "/images/tutorials/6/set-trigram.png" "The hashtags" %}}

_Note: Refer to the [previous tutorial]({{< relref "tutorial-5/index.md" >}}) if you're not sure about creating an index on a string predicate._

Now, let's re-run the `regexp` query.

{{% load-img "/images/tutorials/6/regex-query-1.png" "regex-1" %}}

_Note: Refer to [the first tutorial]({{< relref "tutorial-1/index.md" >}}) if you're not familiar with the query structure in general_
Success!

But we only have the following hashtags in the result: `Dgraph` and `graphqlconf`.

That's because `regexp` function is case-sensitive by default.

Add the character `i` at the the end of the second argument of the `regexp` function to make it case insensitive: `regexp(predicate, /regular-expression/i)`

{{% load-img "/images/tutorials/6/regex-query-2.png" "regex-2" %}}

Now we have the four hashtags with substring `graph` in them.

Let's modify the regular expression to match only the `hashtags` which have a prefix called `graph`.

```graphql
{
  reg_search(func: regexp(hashtag, /^graph.*$/i)) {
    hashtag
  }
}
```

{{% load-img "/images/tutorials/6/regex-query-3.png" "regex-3" %}}

## Summary

In this tutorial, we learned about Full-text search and regular expression based search capabilities in Dgraph.

Did you know that Dgraph also offers fuzzy search capabilities, which can be used to power features like `product` search in an e-commerce store?

Let's learn about the fuzzy search in our next tutorial.

Sounds interesting?

Check out our next tutorial of the getting started series [here]({{< relref "tutorial-7/index.md" >}}).

## Need Help

* Please use [discuss.dgraph.io](https://discuss.dgraph.io) for questions, feature requests, bugs, and discussions.
