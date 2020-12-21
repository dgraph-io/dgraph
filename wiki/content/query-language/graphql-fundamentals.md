+++
date = "2017-03-20T22:25:17+11:00"
title = "GraphQL+- Fundamentals"
weight = 1   
[menu.main]
    parent = "query-language"
+++

Dgraph's GraphQL+- is based on Facebook's [GraphQL](https://facebook.github.io/graphql/).  GraphQL wasn't developed for Graph databases, but its graph-like query syntax, schema validation and subgraph shaped response make it a great language choice.  We've modified the language to better support graph operations, adding and removing features to get the best fit for graph databases.  We're calling this simplified, feature rich language, ''GraphQL+-''.

GraphQL+- is a work in progress. We're adding more features and we might further simplify existing ones.

## Take a Tour - https://dgraph.io/tour/

This document is the Dgraph query reference material.  It is not a tutorial.  It's designed as a reference for users who already know how to write queries in GraphQL+- but need to check syntax, or indices, or functions, etc.

{{% notice "note" %}}If you are new to Dgraph and want to learn how to use Dgraph and GraphQL+-, take the tour - https://dgraph.io/tour/{{% /notice %}}


### Running examples

The examples in this reference use a database of 21 million triples about movies and actors.  The example queries run and return results.  The queries are executed by an instance of Dgraph running at https://play.dgraph.io/.  To run the queries locally or experiment a bit more, see the [Getting Started]({{< relref "get-started/index.md" >}}) guide, which also shows how to load the datasets used in the examples here.

## Queries

A GraphQL+- query finds nodes based on search criteria, matches patterns in a graph and returns a graph as a result.

A query is composed of nested blocks, starting with a query root.  The root finds the initial set of nodes against which the following graph matching and filtering is applied.

{{% notice "note" %}}See more about Queries in [Queries design concept]({{< relref "design-concepts/concepts.md#queries" >}}) {{% /notice %}}

## Returning Values

Each query has a name, specified at the query root, and the same name identifies the results.

If an edge is of a value type, the value can be returned by giving the edge name.

Query Example: In the example dataset, edges that link movies to directors and actors, movies have a name, release date and identifiers for a number of well known movie databases.  This query, with name `bladerunner`, and root matching a movie name, returns those values for the early 80's sci-fi classic "Blade Runner".

{{< runnable >}}
{
  bladerunner(func: eq(name@en, "Blade Runner")) {
    uid
    name@en
    initial_release_date
    netflix_id
  }
}
{{< /runnable >}}

The query first searches the graph, using indexes to make the search efficient, for all nodes with a `name` edge equaling "Blade Runner".  For the found node the query then returns the listed outgoing edges.

Every node had a unique 64-bit identifier.  The `uid` edge in the query above returns that identifier.  If the required node is already known, then the function `uid` finds the node.

Query Example: "Blade Runner" movie data found by UID.

{{< runnable >}}
{
  bladerunner(func: uid(0x394c)) {
    uid
    name@en
    initial_release_date
    netflix_id
  }
}
{{< /runnable >}}

A query can match many nodes and return the values for each.

Query Example: All nodes that have either "Blade" or "Runner" in the name.

{{< runnable >}}
{
  bladerunner(func: anyofterms(name@en, "Blade Runner")) {
    uid
    name@en
    initial_release_date
    netflix_id
  }
}
{{< /runnable >}}

Multiple IDs can be specified in a list to the `uid` function.

Query Example:
{{< runnable >}}
{
  movies(func: uid(0xb5849, 0x394c)) {
    uid
    name@en
    initial_release_date
    netflix_id
  }
}
{{< /runnable >}}


{{% notice "note" %}} If your predicate has special characters, then you should wrap it with angular
brackets while asking for it in the query. E.g. `<first:name>`{{% /notice %}}

## Expanding Graph Edges

A query expands edges from node to node by nesting query blocks with `{ }`.

Query Example: The actors and characters played in "Blade Runner".  The query first finds the node with name "Blade Runner", then follows  outgoing `starring` edges to nodes representing an actor's performance as a character.  From there the `performance.actor` and `performance.character` edges are expanded to find the actor names and roles for every actor in the movie.
{{< runnable >}}
{
  brCharacters(func: eq(name@en, "Blade Runner")) {
    name@en
    initial_release_date
    starring {
      performance.actor {
        name@en  # actor name
      }
      performance.character {
        name@en  # character name
      }
    }
  }
}
{{< /runnable >}}


## Comments

Anything on a line following a `#` is a comment

## Applying Filters

The query root finds an initial set of nodes and the query proceeds by returning values and following edges to further nodes - any node reached in the query is found by traversal after the search at root.  The nodes found can be filtered by applying `@filter`, either after the root or at any edge.

Query Example: "Blade Runner" director Ridley Scott's movies released before the year 2000.
{{< runnable >}}
{
  scott(func: eq(name@en, "Ridley Scott")) {
    name@en
    initial_release_date
    director.film @filter(le(initial_release_date, "2000")) {
      name@en
      initial_release_date
    }
  }
}
{{< /runnable >}}

Query Example: Movies with either "Blade" or "Runner" in the title and released before the year 2000.

{{< runnable >}}
{
  bladerunner(func: anyofterms(name@en, "Blade Runner")) @filter(le(initial_release_date, "2000")) {
    uid
    name@en
    initial_release_date
    netflix_id
  }
}
{{< /runnable >}}

## Language Support

{{% notice "note" %}}A `@lang` directive must be specified in the schema to query or mutate
predicates with language tags.{{% /notice %}}

Dgraph supports UTF-8 strings.

In a query, for a string valued edge `edge`, the syntax
```
edge@lang1:...:langN
```
specifies the preference order for returned languages, with the following rules.

* At most one result will be returned (except in the case where the language list is set to *).
* The preference list is considered left to right: if a value in given language is not found, the next language from the list is considered.
* If there are no values in any of the specified languages, no value is returned.
* A final `.` means that a value without a specified language is returned or if there is no value without language, a value in ''some'' language is returned.
* Setting the language list value to * will return all the values for that predicate along with their language. Values without a language tag are also returned.

For example:

- `name`   => Look for an untagged string; return nothing if no untagged value exits.
- `name@.` => Look for an untagged string, then any language.
- `name@en` => Look for `en` tagged string; return nothing if no `en` tagged string exists.
- `name@en:.` => Look for `en`, then untagged, then any language.
- `name@en:pl` => Look for `en`, then `pl`, otherwise nothing.
- `name@en:pl:.` => Look for `en`, then `pl`, then untagged, then any language.
- `name@*` => Look for all the values of this predicate and return them along with their language. For example, if there are two values with languages en and hi, this query will return two keys named "name@en" and "name@hi".


{{% notice "note" %}}

In functions, language lists (including the `@*` notation) are not allowed.
Untagged predicates, Single language tags, and `.` notation work as described
above.

---

In [full-text search functions]({{< relref "query-language/functions.md#full-text-search" >}})
(`alloftext`, `anyoftext`), when no language is specified (untagged or `@.`),
the default (English) full-text tokenizer is used. This does not mean that
the value with the `en` tag will be searched when querying the untagged value,
but that untagged values will be treated as English text. If you don't want that
to be the case, use the appropriate tag for the desired language, both for
mutating and querying the value.

{{% /notice %}}


Query Example: Some of Bollywood director and actor Farhan Akhtar's movies have a name stored in Russian as well as Hindi and English, others do not.

{{< runnable >}}
{
  q(func: allofterms(name@en, "Farhan Akhtar")) {
    name@hi
    name@en

    director.film {
      name@ru:hi:en
      name@en
      name@hi
      name@ru
    }
  }
}
{{< /runnable >}}

