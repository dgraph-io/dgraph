+++
title = "Deploying on Slash GraphQL"
[menu.main]
    parent = "todo-app-tutorial"
    weight = 6   
+++

*You can join the waitlist for Slash GraphQL [here](https://dgraph.io/slash-graphql).*

Let's now deploy our fully functional app on Slash GraphQL [slash.dgraph.io](https://slash.dgraph.io).

### Create a deployment

After successfully logging into the site for the first time, your dashboard should look something like this.

![Slash-GraphQL: Get Started](/images/graphql/tutorial/todo/slash-graphql-1.png)

Let's go ahead and launch a new deployment.

![Slash-GraphQL: Create deployment](/images/graphql/tutorial/todo/slash-graphql-2.png)

We named our deployment `todo-app-deployment` and set the optional subdomain as
`todo-app`, using which the deployment will be accessible. We can choose any
subdomain here as long as it is available.

Let's set it up in AWS, in the US region, and click on the *Launch* button.

![Slash-GraphQL: Deployment created ](/images/graphql/tutorial/todo/slash-graphql-3.png)

Now the backend is ready.

![Slash-GraphQL: Add an API Key ](/images/graphql/tutorial/todo/slash-graphql-4.png)

An API key can be created from the settings page. Keep in mind to copy the API key once it is created, as it won't be accessible after that.

![Slash-GraphQL: Select API Key role ](/images/graphql/tutorial/todo/slash-graphql-5.png)

There are two types of API keys, client and admin. A client API key can only be used to perform query, mutation and commit operations. An admin API key can be used to perform both client operations and admin operations like drop data, destroy backend, update schema.

Once the deployment is ready, let's add our schema there (insert your public key) by going to the schema tab.

```graphql
type Task @auth(
    query: { rule: """
        query($USER: String!) {
            queryTask {
                user(filter: { username: { eq: $USER } }) {
                    __typename
                }
            }
        }"""}), {
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
# Dgraph.Authorization X-Auth0-Token https://dgraph.io/jwt/claims RS256 "<AUTH0-APP-PUBLIC-KEY>"
```

Once the schema is submitted successfully, we can use the GraphQL API endpoint.

Let's update our frontend to use this URL instead of localhost. Open `src/config.json` and update the `graphqlUrl` field with your GraphQL API endpoint.

```json
{
    ...
    "graphqlUrl": "<Slash-GraphQL-API>"
}
```

That's it! Just in two steps on Slash GraphQL (deployment & schema), we got a GraphQL API that we can now easily use in any application!
