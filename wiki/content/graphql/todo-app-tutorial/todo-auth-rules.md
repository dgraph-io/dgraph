+++
title = "Auth Rules"
weight = 4
[menu.main]
    parent = "todo-app-tutorial"
+++

In the current state of the app, we can view anyone's todos, but we want our todos to be private to us. Let's do that using the `auth` directive to limit that to the user's todos.

We want to limit the user to its own todos, so we will define the query in `auth` to filter depending on the user's username.

Let's update the schema to include that, and then let's understand what is happening there -

```graphql
type Task @auth(
    query: { rule: """
        query($USER: String!) {
            queryTask {
                user(filter: { username: { eq: $USER } }) {
                    __typename
                }
            }
        }"""}){
    id: ID!
    title: String! @search(by: [fulltext])
    completed: Boolean! @search
    user: User!
}
type User {
  username: String! @id @search(by: [hash])
  name: String
  tasks: [Task] @hasInverse(field: user)
}
```

Resubmit the updated schema -
```
curl -X POST localhost:8080/admin/schema --data-binary '@schema.graphql'
```

Now let's see what does the definition inside the `auth` directive means. Firstly, we can see that this rule applies to `query` (similarly we can define rules on `add`, `update` etc.). 

```graphql
 query ($USER: String!) {
  queryTask {
    user(filter: {username: {eq: $USER}}) {
      __typename
    }
  }
}
```

The rule contains a parameter `USER` which we will use to filter the todos by a user. As we know `queryTask` returns an array of `task` that contains the `user` also and we want to filter it by `user`, so we compare the `username` of the user with the `USER` passed to the auth rule (logged in user). 
 
Now the next thing you would be wondering is that how do we pass a value for the `USER` parameter in the auth rule since its not something that you can call, the answer is pretty simple actually that value will be extracted from the JWT token which we pass to our GraphQL API as a header and then it will execute the rule. 

Let's see how we can do that in the next step using Auth0 as an example.
