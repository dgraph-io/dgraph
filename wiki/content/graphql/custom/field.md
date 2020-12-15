+++
title = "Custom Fields"
weight = 5
[menu.main]
    parent = "custom"
+++

Custom fields allow you to extend your types with custom logic as well as make joins between your local data and remote data.

Let's say we are building an app for managing projects.  Users will login with their GitHub id and we want to connect some data about their work stored in Dgraph with say their GitHub profile, issues, etc.

Our first version of our users might start out with just their GitHub username and some data about what projects they are working on.

```graphql
type User {
    username: String! @id 
    projects: [Project]
    tickets: [Ticket]
}
```

We can then add their GitHub repositories by just extending the definitions with the types and custom field needed to make the remote call.

```graphql
# GitHub's repository type
type Repository @remote { ... }

# Dgraph user type
type User {
    # local user name = GitHub id
    username: String! @id 

    # join local data with remote
    repositories: [Repository] @custom(http: {
        url:  "https://api.github.com/users/$username/repos",
        method: GET
    })
}
```

We could similarly join with say the GitHub user details, or open pull requests, to further fill out the join between GitHub and our local data.  Instead of the REST API, let's use the GitHub GraphQL endpoint


```graphql
# GitHub's User type
type GitHubUser @remote { ... }

# Dgraph user type
type User {
    # local user name = GitHub id
    username: String! @id 

    # join local data with remote
    gitDetails: GitHubUser @custom(http: {
        url:  "https://api.github.com/graphql",
        method: POST,
        graphql: "query(username: String!) { user(login: $username) }",
        skipIntrospection: true
    })
}
```

Perhaps our app has some measure of their volocity that's calculated by a custom function that looks at both their GitHub commits and some other places where work is added.  Soon we'll have a schema where we can render a user's home page, the projects they work on, their open tickets, their GitHub details, etc. in a single request that queries across multiple sources and can mix Dgraph filtering with external calls.

```graphql
query {
    getUser(id: "aUser") {
        username
        projects(order: { asc: lastUpdate }, first: 10) {
            projectName
        }
        tickets { 
            connectedGitIssue { ... }
        }
        velocityMeasure
        gitDetails { ... }
        repositories { ... }
    }
}
```

---
