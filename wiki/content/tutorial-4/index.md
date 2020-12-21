+++
title = "Get Started with Dgraph - Multi-language strings"
+++

**Welcome to the fourth tutorial of getting started with Dgraph.**

In the [previous tutorial]({{< relref "tutorial-3/index.md" >}}), we learned about Datatypes, Indexing, Filtering, and Reverse traversals in Dgraph.

In this tutorial, we'll learn about using multi-language strings and operations on them using the language tags.

You can see the accompanying video below.

{{< youtube _lDE9QXHZC0 >}}

---

## Strings and languages

Strings values in Dgraph are of UTF-8 format.
Dgraph also supports values for string predicate types in multiple languages.
The multi-lingual capability is particularly useful to build features, which requires you to store the same information in multiple languages.

Let's learn more about them!

Let's start with building a simple food review Graph.
Here's the Graph model.

{{% load-img "/images/tutorials/4/a-graph-model.jpg" "model" %}}

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
_Note: If this mutation syntax is new to you, refer to the [first tutorial]({{< relref "tutorial-1/index.md">}}) to learn basics of mutation in Dgraph._

Here's our Graph!

{{% load-img "/images/tutorials/4/a-full-graph.png" "full graph" %}}

Our Graph has:

- Five blue food nodes.
- The green nodes represent the country of origin of these food items.
- The reviews of the food items are in pink.

You can also see that Dgraph has auto-detected the data types of the predicates.
You can check that out from the schema tab.

{{% load-img "/images/tutorials/4/c-schema.png" "full graph" %}}

_Note: Check out the [previous tutorial]({{< relref "tutorial-3/index.md">}}) to know more about data types in Dgraph._

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

_Note: Check the [second tutorial]({{< relref "tutorial-2/index.md">}}) if you want to learn more about traversal queries like the above one_

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

{{% load-img "/images/tutorials/4/b-comments.png" "full graph" %}}

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

_Note: If you're not clear about basic terminology like `predicates`, do read the [first tutorial]({{< relref "tutorial-1/index.md">}})._

Let's run the above mutation.

Go to the mutate tab, paste the mutation, and click Run.

{{% load-img "/images/tutorials/4/d-lang-error.png" "lang error" %}}

We got an error! Using the language tag requires you to add the `@lang` directive to the schema.

Follow the instructions below to add the `@lang` directive to the `comment` predicate.

- Go to the Schema tab.
- Click on the `comment` predicate.
- Tick mark the `lang` directive.
- Click on the `Update` button.

{{% load-img "/images/tutorials/4/e-update-lang.png" "lang error" %}}

Let's re-run the mutation.

{{% load-img "/images/tutorials/4/f-mutation-success.png" "lang error" %}}

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

## Querying using language tags.

Let's obtain the review comments only for `Sushi`.

In the [previous article]({{< relref "tutorial-3/index.md">}}), we learned about using the `eq` operator and the `hash` index to query for string predicate values.

Using that knowledge, let's first add the `hash` index for the `food_name` predicate.

{{% load-img "/images/tutorials/4/g-hash.png" "hash index" %}}

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

{{% load-img "/images/tutorials/4/h-comment.png" "hash index" %}}

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

{{% load-img "/images/tutorials/4/i-japanese.png" "Japanese" %}}

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

{{% load-img "/images/tutorials/4/j-russian.png" "Russian" %}}

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

{{% load-img "/images/tutorials/4/k-star.png" "Russian" %}}

Here is the table with the syntax for various ways of making use of language tags while querying.

| Syntax | Result |
|------------ |--------|
| comment | Look for an untagged string; return nothing if no untagged review exists. |
| comment@. | Look for an untagged string, if not found, then return review in any language. But, this returns only a single value. |
| comment@jp | Look for comment tagged `@jp`. If not found, the query returns nothing.|
| comment@ru | Look for comment tagged `@ru`. If not found, the query returns nothing. |
| comment@jp:. | Look for comment tagged `@jp` first. If not found, then find the untagged comment. If that's not found too, return anyone comment in other languages. |
| comment@jp:ru | Look for comment tagged `@jp`, then `@ru`. If neither is found, it returns nothing. |
| comment@jp:ru:. | Look for comment tagged `@jp`, then `@ru`. If both not found, then find the untagged comment. If that's not found too, return any other comment if it exists. |
| comment@* | Return all the language tags, including the untagged. |

If you remember, we had initially added a Russian dish `Borscht` with its review in `Russian`.

{{% load-img "/images/tutorials/4/l-russian.png" "Russian" %}}

If you notice, we haven't used the language tag `@ru` for the review written in Russian.

Hence, if we query for all the reviews written in `Russian`, the review for `Borscht` doesn't make it to the list.

Only the review for `Sushi,` written in `Russian`, makes it to the list.

{{% load-img "/images/tutorials/4/m-sushi.png" "Russian" %}}

So, here's the lesson of the day!

> If you are representing the same information in different languages, don't forget to add your language tags!

## Summary

In this tutorial, we learned about using multi-language string and operations on them using the language tags.

The usage of tags is not just restricted to multi-lingual strings.
Language tags are just a use case of Dgraph's capability to tag data.

In the next tutorial, we'll continue our quest into the string types in Dgraph.
We'll explore the string type indices in detail.

Sounds interesting?

Check out our next tutorial of the getting started series [here]({{< relref "tutorial-5/index.md" >}}).

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
