+++
title = "Cascade"
weight = 5
[menu.main]
    parent = "graphql-queries"
    name = "Cascade"
+++

`@cascade` is available as a directive that can be applied to fields. With the @cascade
directive, nodes that don’t have all fields specified in the query are removed.
This can be useful in cases where some filter was applied and some nodes might not
have all the listed fields.

For example, the query below only return the authors which have both reputation
and posts, where posts have text. Note that `@cascade` trickles down so it would
automatically be applied at the `posts` level as well if it's applied at the `queryAuthor`
level.

```graphql
{
    queryAuthor @cascade {
        reputation
        posts {
            text
        }
    }
}
```

`@cascade` can also be used at nested levels, so the query below would return all authors
but only those posts which have both `text` and `id`.

```graphql
{
    queryAuthor  {
        reputation
        posts @cascade {
            id
            text
        }
    }
}
```

### Parameterized Cascade

@cascade can also optionally take a list of fields as an argument. This changes the default behaviour to consider only the supplied fields as mandatory instead of all the fields for a type.

In the example below, name is supplied in the fields argument. Listed fields are also automatically cascaded as a required argument to nested selection sets. 
For example, name is supplied in the fields argument below, so for an author to be in the query response it must have a name and if it has a subfield country, then that must also have name.
```graphql
{
    queryAuthor  @cascade(fields:["name"]) {
        reputation
        name
        country{
           Id
           name
        }
    }
}
```
The query below only return those posts which have a non-null text field.
```graphql
{
        queryAuthor {
		reputation
		name
		posts @cascade(fields:["text"]) {
		   title
		   text
		}
	}
}
```
The cascading nature of field selection is overwritten by a nested @casecade. 
For example, the query below ensures that an author has fields reputation and name, and, if it has subfield posts, then that must have field text.

```graphql
{
      queryAuthor @cascade(fields:["reputation","name"]) {
        reputation
        name
        dob
        posts @cascade(fields:["text"]) {
            title
            text
            }
        }
}
```
