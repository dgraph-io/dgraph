+++
title = "Authentication"
weight = 2   
[menu.main]
    parent = "slash-graphql-admin"
+++

All the APIs documented here require an API token for access. A new API token can be generated from Slash GraphQL by selecting the ["Settings" button](https://slash.dgraph.io/_/settings) from the sidebar, then clicking the Add API Key button. Keep your API key safe, it will not be accessible once you leave the page.

![Slash-GraphQL: Add an API Key ](/images/graphql/tutorial/todo/slash-graphql-4.png)

There are two types of API keys, client and admin. 
- **Client API keys** can only be used to perform query, mutation and commit operations.
- **Admin API keys** can be used to perform both client operations and admin operations like drop data, destroy backend, update schema.

<img src="/images/graphql/tutorial/todo/slash-graphql-5.png" alt="Select API Key Role" width="60%">
<br>
<br>
All admin API requests must be authenticated by passing the API token as the 'X-Auth-Token' header to every HTTP request. You can verify that your API token works by using the following HTTP example.

```
curl 'https://<your-backend>/admin' \
  -H 'X-Auth-Token: <your-token>' \
  -H 'Content-Type: application/json' \
  --data-binary '{"query":"{ getGQLSchema { schema } }"}'
```
