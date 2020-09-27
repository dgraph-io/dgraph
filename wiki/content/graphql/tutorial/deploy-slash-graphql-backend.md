+++
title = "Slash GraphQL"
[menu.main]
    parent = "build-an-app-tutorial"
    identifier = "deploy-slash-graphql-backend"
    weight = 2   
+++

*In this section: you'll set up a Slash GraphQL account and learn how to deploy a GraphQL backend.*

We are going to build a serverless GraphQL app using Slash GraphQL as the backend, so let's start by setting ourselves up with an account and deploying a backend ready to use for building our app.

## Deploy a free GraphQL backend

To use Slash GraphQL, you'll need to get an account --- for this tutorial we'll use the free plan.

You can learn some more about Slash GraphQL [here](https://dgraph.io/slash-graphql) or on our [docs](https://dgraph.io/docs/slash-graphql/introduction/).

Get started by signing up for Slash GraphQL [here](https://slash.dgraph.io).  You'll need to verify your email before you can continue to use Slash GraphQL.

Once you have signed up and verified your signup email address, log into Slash GraphQL and you'll arrive at this screen.

![Graph schema sketch](/images/graphql/tutorial/discuss/slash-first-login.png)

Because you don't have any GraphQL backends yet, you won't have a visible dashboard.  Click the "Launch New Backend" button and you'll be taken to a screen to enter the details of the backend.

Name the backed, optionally set a subdomain (if left blank, Slash GraphQL picks a random domain for you), and pick a region to deploy you GraphQL backend to.  For this tutorial leave the free tier selected.

![Graph schema sketch](/images/graphql/tutorial/discuss/slash-launch-backend.png)

Once you have the settings, click "Launch" and your backend will be deployed.  It takes a few seconds to deploy the infrastructure, but soon you'll have a backed ready for the tutorial.

![Graph schema sketch](/images/graphql/tutorial/discuss/slash-backend-live.png)

That's it!  You now have a running GraphQL backend.

## What's next

We'll now move on to the design process - it's graph-first, in fact, it's GraphQL-first. We'll design the GraphQL types that our app is based around, learn about how graphs work and then look at some example queries and mutations.