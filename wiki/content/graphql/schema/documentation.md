+++
title = "Documentation and Comments"
weight = 6
[menu.main]
    parent = "schema"
+++

## Schema Documentation Processed  by Generated API
Dgraph accepts GraphQL documentation comments (e.g. `""" This is a graphql comment """`), which get passed through to the generated API and thus shown as documentation in GraphQL tools like GraphiQL, GraphQL Playground, Insomnia etc.

## Schema Documentation Ignored by Generated API
You can also add `# ...` comments where ever you like.  These comments are not passed via the generated API and are not visible in the API docs.

## Reserved Namespace in Dgraph
Any comment starting with `# Dgraph.` is **reserved** and **should not be used** to document your input schema.

## An Example
An example that adds comments to a type as well as fields within the type would be as below.

```graphql
"""
Author of questions and answers in a website
"""
type Author {
# ... username is the author name , this is an example of a dropped comment
  username: String! @id
"""
The questions submitted by this author
"""
  questions: [Question] @hasInverse(field: author)
"""
The answers submitted by this author
"""
  answers: [Answer] @hasInverse(field: author)
}
```

It is also possible to add comments for queries or mutations that have been added via the custom directive.
```graphql
type Query {
"""
This query involves a custom directive, and gets top authors.
"""
getTopAuthors(id: ID!): [Author] @custom(http: {
    url: "http://api.github.com/topAuthors",
    method: "POST",
    introspectionHeaders: ["Github-Api-Token"],
    secretHeaders: ["Authorization:Github-Api-Token"]
  })
}
```
The screenshots below shows how the documentation appear in a Grapqhl API explorer.<br>

{{% load-img "/images/graphql/authors1.png" "Schema Documentation On Types" %}}
<p style="text-align: left;">Schema Documentation on Types</p>
<br>
{{% load-img "/images/graphql/CustomDirectiveDocumentation.png" "Schema Documentation On Custom Directive" %}}
<p style="text-align: left;">Schema Documentation on Custom directive</p>

