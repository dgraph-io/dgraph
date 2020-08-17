+++
title = "Authentication"
[menu.main]
    parent = "slash-graphql-admin"
    weight = 2   
+++

All the APIs documented here require an API token for access. A new API token can be generated from Slash GraphQL by selecting the ["Settings" button](https://slash.dgraph.io/_/settings) from the sidebar, then clicking the Add API Key button. Keep your API key safe, it will not be accessible once you leave the page.

All admin API requests must be authenticated by passing the API token as the 'X-Auth-Token' header to every HTTP request. You can verify that your API token works by using the following HTTP example.

```
curl 'https://<your-backend>/admin' \
  -H 'X-Auth-Token: <your-token>' \
  -H 'Content-Type: application/json' \
  --data-binary '{"query":"{ getGQLSchema { schema } }"}'
```
