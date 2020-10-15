+++
title = "One-click Deploy"
weight = 2
[menu.main]
    parent = "slash-graphql"
+++

We love building GraphQL apps at Dgraph! In order to help you get started quickly, we are making it easy to launch a few apps with a single click. We'll create a fresh backend for you, load it with the right schema, and even launch a front end that's ready for you to use.

In order to do a `One-Click Deploy`, please choose the application you wish to deploy from the [Apps](https://slash.dgraph.io/_/one-click) tab in the side bar and click the `Deploy` button.

It will take few minutes while the new back-end spins up. You will be able to see the front-end url under `Details` card on the [Dashboard](https://slash.dgraph.io/_/dashboard) tab in the sidebar.

## Deploying to your own domain.

If you wish to deploy your apps to your own domain, you can do that easily with any of the hosting services. You can follow below steps to deploy your app on [Netlify](https://www.netlify.com/).

1. Fork the github repo of the app you wish to deploy.
2. Link your forked repo to Netlify by Clicking `New Site from Git` and
   {{% load-img "/images/importSite.png" "Import Site" %}}
3. After successful import of the forked repository click on `Show advanced` and `New variable`.
4. Add `REACT_APP_GRAPHQL_ENDPOINT` in the key and your graphql endpoint obtained from Slash GraphQL in the value field.
5. Click `Deploy Site` on Netlify Dashboard.
   {{% load-img "/images/advanced-SettingsNetlify.png" "Advanced Settings" %}}
6. You can configure [Auth0](https://auth0.com/) steps by following Authorization section of the blogpost [here.](https://dgraph.io/blog/post/surveyo-into/)
