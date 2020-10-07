+++
title = "Mutations"
weight = 3
[menu.main]
    parent = "authorization"
+++

Mutations with auth work similarly to query.  However, mutations involve a state change in the database, so it's important to understand when the rules are applied and what they mean.

## Add

Rules for `add` authorization state that the rule must hold of nodes created by the mutation data once committed to the database.

For example, a rule such as:

```graphql
type Todo @auth(
    add: { rule: """
        query ($USER: String!) { 
            queryTodo {
                owner(filter: { username: { eq: $USER } } ) { 
                    username
                } 
            } 
        }"""
    }
){
    id: ID!
    text: String!
    owner: User
}
type User {
    username: String! @id
    todos: [Todo]
}
```

states that if you add a new todo, then that new todo must be a todo that satisfies the `add` rule, in this case saying that you can only add todos with yourself as the author.

## Delete

Delete rules filter the nodes that can be deleted.  A user can only ever delete a subset of the nodes that the `delete` rules allow.  

For example, this rule states that a user can delete a todo if they own it, or they have the `ADMIN` role.

```graphql
type Todo @auth(
    delete: { or: [ 
        { rule: """
            query ($USER: String!) { 
                queryTodo {
                    owner(filter: { username: { eq: $USER } } ) { 
                        username
                    } 
                } 
            }"""
        },
        { rule:  "{$ROLE: { eq: \"ADMIN\" } }"}
    ]}
){
    id: ID!
    text: String! @search(by: [term])
    owner: User
}

type User {
    username: String! @id
    todos: [Todo]
}
```

So a mutation like:

```graphql
mutation {
    deleteTodo(filter: { text: { anyofterms: "graphql" } }) {
        numUids    
    }
}
```

for most users would delete their own posts that contain the term "graphql", but wouldn't affect any other user's todos; for an admin, it would delete any users posts that contain "graphql"

For add, what matters is the resulting state of the database, for delete it's the state before the delete occurs.

## Update

Updates have both a before and after state that can be important for auth.  

For example, consider a rule stating that you can only update your own todos.  If evaluated in the database before the mutation, like the delete rules, it would prevent you updating anyone elses todos, but does it stop you updating your own todo to have a different `owner`.  If evaluated in the database after the mutation occurs, like for add rules, it would stop setting the `owner` to another user, but would it prevent editing other's posts.

Currently, Dgraph evaluates `update` rules _before_ the mutation.  Our auth support is still in beta and we may extend this for example to make the `update` rule an invariant of the mutation, or enforce pre and post conditions, or even allow custom logic to validate the update data.

## Update and Add

Update mutations can also insert new data.  For example, you might allow a mutation that runs an update mutation to add a new todo.

```graphql
mutation {
    updateUser(input: {
        filter: { username: { eq: "aUser" }},
        set: { todos: [ { text: "do this new todo"} ] }
    }) {
        ...
    }
}
```

Such a mutation updates a user's todo list by inserting a new todo.  It would have to satisfy the rules to update the author _and_ the rules to add a todo.  If either fail, the mutation has no effect.

---