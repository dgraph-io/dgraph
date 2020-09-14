+++
title = "Cascade"
[menu.main]
    parent = "graphql-queries"
    name = "Cascade"
    weight = 5   
+++

`@cascade` is available as a directive which can be applied on fields. With the @cascade
directive, nodes that don’t have all fields specified in the query are removed.
This can be useful in cases where some filter was applied and some nodes might not
have all listed fields.

For example, the query below would only return the authors which have both reputation
and posts and where posts have text. Note that `@cascade` trickles down so it would
automatically be applied at the `posts` level as well if its applied at the `queryAuthor`
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

@cascade is sometimes very strict because for a node to be in response, all its children in the sub-graph must be present. 
Instead to consider all fields in cascade, we give flexiblity to user to specify fields as argument to cascade as below.

`@cascade(fields:["field1","field2"..])` 

In below example argument field "name" is used as argument to cascade and forwarded to next query level also. So for author 
to be in query response it should have name and if has subfield country that should also have name.
```graphql
queryAuthor  @cascade(fields:["name"]) {
		reputation
		name
		country {
                Id
				name
		}
	}
```
Following query ensures authors which has post field should also have text subfield in it.
```graphql
queryAuthor {
		reputation
		name
		posts @cascade(fields:["text"]) {
			   title
			   text
		}
	}
```
Also these fields are  forwarded to next query level unless there is a cascade at next level , 
in that case arguments to cascade are overwritten.
Below query ensures that  author should have fields reputation and name. And if it has subfield posts then it should also have
field text in it.

```graphql
queryAuthor @cascade(fields:["reputation","name"]) {
		reputation
		name
		dob
		posts @cascade(fields:["text"]) {
				title
				text
			 }
		}
```
