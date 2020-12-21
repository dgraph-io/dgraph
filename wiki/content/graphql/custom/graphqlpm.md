+++
title = "Custom DQL"
weight = 6
[menu.main]
    parent = "custom"
+++

At present, it is an experimental feature in master. You can specify the DQL (aka GraphQL+-) query you want to execute
while running a custom GraphQL query, and Dgraph's GraphQL API will execute that for you.

It helps to build logic that you can't do with the current GraphQL CRUD API.

For example, lets say you had following schema:
```graphql
type Tweets {
	id: ID!
	text: String! @search(by: [fulltext])
	author: User
	timestamp: DateTime! @search
}
type User {
	screen_name: String! @id
	followers: Int @search
	tweets: [Tweets] @hasInverse(field: author)
}
```

and you wanted to query tweets containing some particular text sorted by the number of followers their author has. Then,
this is not possible with the automatically generated CRUD API. Similarly, let's say you have a table sort of UI
component in your application which displays only a user's name and the number of tweets done by that user. Doing this
with the auto-generated CRUD API would require you to fetch unnecessary data at client side, and then employ client side
logic to find the count. Instead, all this could simply be achieved by specifying a DQL query for such custom use-cases.

So, you would need to modify your schema like this:
```graphql
type Tweets {
	id: ID!
	text: String! @search(by: [fulltext])
	author: User
	timestamp: DateTime! @search
}
type User {
	screen_name: String! @id
	followers: Int @search
	tweets: [Tweets] @hasInverse(field: author)
}
type UserTweetCount @remote {
	screen_name: String
	tweetCount: Int
}

type Query {
  queryTweetsSortedByAuthorFollowers(search: String!): [Tweets] @custom(dql: """
	query q($search: string) {
		var(func: type(Tweets)) @filter(anyoftext(Tweets.text, $search)) {
			Tweets.author {
				followers as User.followers
			}
			authorFollowerCount as sum(val(followers))
		}
		queryTweetsSortedByAuthorFollowers(func: uid(authorFollowerCount), orderdesc: val(authorFollowerCount)) {
			id: uid
			text: Tweets.text
			author: Tweets.author {
			    screen_name: User.screen_name
			    followers: User.followers
			}
			timestamp: Tweets.timestamp
		}
	}
	""")

  queryUserTweetCounts: [UserTweetCount] @custom(dql: """
	query {
		queryUserTweetCounts(func: type(User)) {
			screen_name: User.screen_name
			tweetCount: count(User.tweets)
		}
	}
	""")
}

```

Now, if you run following query, it would fetch you the tweets containing "GraphQL" in their text, sorted by the number
of followers their author has:
```graphql
query {
    queryTweetsSortedByAuthorFollowers(search: "GraphQL") {
        text
    }
}
```

There are following points to note while specifying the DQL query for such custom resolvers:

* The name of the DQL query that you want to map to the GraphQL response, should be same as the name of the GraphQL query.
* You must use proper aliases inside DQL queries to map them to the GraphQL response.
* If you are using variables in DQL queries, their names should be same as the name of the arguments for the GrapqhQL query.
* For variables, only scalar GraphQL arguments like Boolean, Int, Float etc are allowed. Lists and Object types are not allowed to be used as variables with DQL queries.
* You would be able to query only those many levels with GraphQL which you have mapped with the DQL query. For instance, in the first custom query above, we haven't mapped an author's tweets to GraphQL alias, so, we won't be able to fetch author's tweets using that query.
* If the custom GraphQL query returns an interface, and you want to use `__typename` in GraphQL query, then you should add `dgraph.type` as a field in DQL query without any alias. This is not required for types, only for interfaces.

---