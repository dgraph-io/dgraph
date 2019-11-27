+++
title = "Get Started with Dgraph - Multi-language strings"
+++

**Welcome to the fourth tutorial of getting started with Dgraph.**

In the [previous tutorial](/tutorial-3/), we learned about Datatypes, Indexing, Filtering, and Reverse traversals in Dgraph.

In this tutorial, we'll learn about using multi-language strings and operations on them using the language tags.

The accompanying video of the tutorial will be out shortly, so stay tuned to [our YouTube channel](https://www.youtube.com/channel/UCghE41LR8nkKFlR3IFTRO4w).

#### Strings and languages
Strings values in Dgraph are of UTF-8 format.
Dgraph also supports values for string predicate types in multiple languages.
The multi-lingual capability is particularly useful to build features, which requires you to store the same information in multiple languages.

Let's learn more about them!

Let's start with building a simple food review Graph.
Here's the Graph model.

![model](/images/tutorials/4/a-graph-model.jpg)

The above Graph has three entities: Food, Comment, and Country.

The nodes in the Graph represent these entities.

For the rest of the tutorial, let's call the node representing a food item as a `food` node.
The node representing a review comment as a `review` node, and the node representing the country of origin as a `country` node.

Here's the relationship between them:

- Every food item is connected to its reviews via the `review` edge.
- Every food item is connected to its country of origin via the `origin` edge.

Let's add some reviews for some fantastic dishes!

How about spicing it up a bit before we do that?

Let's add the reviews for these dishes in the native language of their country of origin.

Let's go, amigos!

```json
{
  "set": [
    {
      "food_name": "Hamburger",
      "review": [
        {
          "comment": "Tastes very good"
        }
      ],
      "origin": [
        {
          "country": "United states of America"
        }
      ]
    },
    {
      "food_name": "Carrillada",
      "review": [
        {
          "comment": "Sabe muy sabroso"
        }
      ],
      "origin": [
        {
          "country": "Spain"
        }
      ]
    },
    {
      "food_name": "Pav Bhaji",
      "review": [
        {
          "comment": "स्वाद बहुत अच्छा है"
        }
      ],
      "origin": [
        {
          "country": "India"
        }
      ]
    },
    {
      "food_name": "Borscht",
      "review": [
        {
          "comment": "очень вкусно"
        }
      ],
      "origin": [
        {
          "country": "Russia"
        }
      ]
    },
    {
      "food_name": "mapo tofu",
      "review": [
        {
          "comment": "真好吃"
        }
      ],
      "origin": [
        {
          "country": "China"
        }
      ]
    }
  ]
}
```
_Note: If this mutation syntax is new to you, refer to the [first tutorial](/tutorial-1/) to learn basics of mutation in Dgraph._

Here's our Graph!

![full graph](/images/tutorials/4/a-full-graph.png)

Our Graph has:

- Five blue food nodes.
- The green nodes represent the country of origin of these food items.
- The reviews of the food items are in pink.

You can also see that Dgraph has auto-detected the data types of the predicates.
You can check that out from the schema tab.

![full graph](/images/tutorials/4/c-schema.png)

_Note: Check out the [previous tutorial](/tutorial-3/) to know more about data types in Dgraph._

Let's write a query to fetch all the food items, their reviews, and their country of origin.

Go to the query tab, paste the query, and click Run.

```graphql
{
  food_review(func: has(food_name)) {
    food_name
      review {
        comment
      }
      origin {
        country
      }
  }
}
```

_Note: Check the [second tutorial](/tutorial-2/) if you want to learn more about traversal queries like the above one_

Now, Let's fetch only the food items and their reviews,

```graphql
{
  food_review(func: has(food_name)) {
    food_name
      review {
        comment
      }
  }
}
```

As expected, these comments are in different languages.

![full graph](/images/tutorials/4/b-comments.png)

But can we fetch the reviews based on their language?
Can we write a query which says: _Hey Dgraph, can you give me only the reviews written in Chinese?_

That's possible, but only if you provide additional information about the language of the string data.
You can do so by using language tags.
While adding the string data using mutations, you can use the language tags to specify the language of the string predicates.

Let's see the language tags in action!

I've heard that Sushi is yummy! Let's add a review for `Sushi` in more than one language.
We'll be writing the review in three different languages: English, Japanese, and Russian.

Here's the mutation to do so.

```json
{
  "set": [
    {
      "food_name": "Sushi",
      "review": [
        {
          "comment": "Tastes very good",
          "comment@jp": "とても美味しい",
          "comment@ru": "очень вкусно"
        }
      ],
      "origin": [
        {
          "country": "Japan"
        }
      ]
    }
  ]
}
```

Let's take a closer look at how we assigned values for the `comment` predicate in different languages.

We used the language tags (@ru, @jp) as a suffix for the `comment` predicate.

In the above mutation:

- We used the `@ru` language tag to add the comment in Russian: `"comment@ru": "очень вкусно"`.

- We used the `@jp` language tag to add the comment in Japanese: `"comment@jp": "とても美味しい"`.

- The comment in `English` is untagged: `"comment": "Tastes very good"`.

In the mutation above, Dgraph creates a new node for the reviews, and stores `comment`, `comment@ru`, and `comment@jp` in different predicates inside the same node.

_Note: If you're not clear about basic terminology like `predicates`, do read the [first tutorial](/tutorial-1/)._

Let's run the above mutation.

Go to the mutate tab, paste the mutation, and click Run.

![lang error](/images/tutorials/4/d-lang-error.png)

We got an error! Using the language tag requires you to add the `@lang` directive to the schema.

Follow the instructions below to add the `@lang` directive to the `comment` predicate.

- Go to the Schema tab.
- Click on the `comment` predicate.
- Tick mark the `lang` directive.
- Click on the `Update` button.

![lang error](/images/tutorials/4/e-update-lang.png)

Let's re-run the mutation.

![lang error](/images/tutorials/4/f-mutation-success.png)

Success!

Again, remember that using the above mutation, we have added only one review for Sushi, not three different reviews!

But, if you want to add three different reviews, here's how you do it.

Adding the review in the format below creates three nodes, one for each of the comments.
But, do it only when you're adding a new review, not to represent the same review in different languages.

```
"review": [
  {
    "comment": "Tastes very good"
  },
  {
    "comment@jp": "とても美味しい"
  },
  {
    "comment@ru": "очень вкусно"
  }
]
```

Dgraph allows any strings to be used as language tags.
But, it is highly recommended only to use the ISO standard code for language tags.

By following the standard, you eliminate the need to communicate the tags to your team or to document it somewhere.
[Click here](https://www.w3schools.com/tags/ref_language_codes.asp) to see the list of ISO standard codes for language tags.

In our next section, let's make use of the language tags in our queries.

#### Querying using language tags.
Let's obtain the review comments only for `Sushi`.

In the [previous article](/tutorial-3/), we learned about using the `eq` operator and the `hash` index to query for string predicate values.

Using that knowledge, let's first add the `hash` index for the `food_name` predicate.

![hash index](/images/tutorials/4/g-hash.png)

Now, go to the query tab, paste the query in the text area, and click Run.

```graphql
{
  food_review(func: eq(food_name,"Sushi")) {
    food_name
      review {
        comment
      }
  }
}
```

![hash index](/images/tutorials/4/h-comment.png)

By default, the query only returns the untagged comment.

But you can use the language tag to query specifically for a review comment in a given language.

Let's query for a review for `Sushi` in Japanese.
```graphql
{
  food_review(func: eq(food_name,"Sushi")) {
    food_name
    review {
      comment@jp
    }
  }
}
```

![Japanese](/images/tutorials/4/i-japanese.png)

Now, let's query for a review for `Sushi` in Russian.

```graphql
{
  food_review(func: eq(food_name,"Sushi")) {
    food_name
    review {
      comment@ru
    }
  }
}
```

![Russian](/images/tutorials/4/j-russian.png)

You can also fetch all the comments for `Sushi` written in any language.

```graphql
{
  food_review(func: eq(food_name,"Sushi")) {
    food_name
    review {
      comment@*
    }
  }
}
```

![Russian](/images/tutorials/4/k-star.png)

Here is the table with the syntax for various ways of making use of language tags while querying.

| Syntax | Result |
|------------ |--------|
| comment | Look for an untagged string; return nothing if no untagged review exists. |
| comment@. | Look for an untagged string, if not found, then return review in any language. But, this returns only a single value. |
| comment@jp | Look for comment tagged `@jp`. If not found, the query returns nothing.|
| comment@ru | Look for comment tagged `@ru`. If not found, the query returns nothing. |
| name@jp:. | Look for comment tagged `@jp` first. If not found, then find the untagged comment. If that's not found too, return anyone comment in other languages. |
| name@jp:ru | Look for comment tagged `@jp`, then `@ru`. If neither is found, it returns nothing. |
| name@jp:ru:. | Look for comment tagged `@jp`, then `@ru`. If both not found, then find the untagged comment. If that's not found too, return any other comment if it exists. |
| name@* | Return all the language tags, including the untagged. |

If you remember, we had initially added a Russian dish `Borscht` with its review in `Russian`.

![Russian](/images/tutorials/4/l-russian.png)

If you notice, we haven't used the language tag `@ru` for the review written in Russian.

Hence, if we query for all the reviews written in `Russian`, the review for `Borscht` doesn't make it to the list.

Only the review for `Sushi,` written in `Russian`, makes it to the list.

![Russian](/images/tutorials/4/m-sushi.png)

So, here's the lesson of the day!

> If you are representing the same information in different languages, don't forget to add your language tags!

#### Summary
In this tutorial, we learned about using multi-language string and operations on them using the language tags.

The usage of tags is not just restricted to multi-lingual strings.
Language tags are just a use case of Dgraph's capability to tag data.

In the next tutorial, we'll continue our quest into the string types in Dgraph.
We'll explore the string type indices in detail.

Sounds interesting?

See you all soon in the next tutorial. Till then, happy Graphing!

## What's Next?
- Go to [Clients]({{< relref "clients/index.md" >}}) to see how to communicate
with Dgraph from your application.
- Take the [Tour](https://tour.dgraph.io) for a guided tour of how to write queries in Dgraph.
- A wider range of queries can also be found in the [Query Language](/query-language) reference.
- See [Deploy](/deploy) if you wish to run Dgraph
  in a cluster.

## Need Help

* Please use [discuss.dgraph.io](https://discuss.dgraph.io) for questions, feature requests and discussions.
* Please use [Github Issues](https://github.com/dgraph-io/dgraph/issues) if you encounter bugs or have feature requests.
* You can also join our [Slack channel](http://slack.dgraph.io).
