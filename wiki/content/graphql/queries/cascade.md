+++
title = "Cascade"
[menu.main]
    parent = "graphql-queries"
    name = "Cascade"
    weight = 5   
+++

`@cascade` is available as a directive that can be applied to fields. With the @cascade
directive, nodes that don’t have all fields specified in the query are removed.
This can be useful in cases where some filter was applied and some nodes might not
have all the listed fields.

For example, the query below would only return the authors which have both reputation
and posts and where posts have text. Note that `@cascade` trickles down so it would
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

In the example below, name is supplied in the fields argument. It would also be automatically cascaded as a required argument to country below. So for author 
to be in query response it should have a name and if has a subfield country then that should also have name.
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
The query below would only return those posts which have a non-null text field.
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
Also, these fields are  forwarded to the next query level unless there is a cascade at the next level, 
in that case, arguments to cascade are overwritten.
The below query ensures that the author should have field reputation and name. And if it has subfield posts then that should also have field text in it.

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
