+++
title = "Introduction"
[menu.main]
    parent = "build-an-app-tutorial"
    identifier = "introduction"
    weight = 1   
+++

Welcome to Slash GraphQL and Dgraph.  

Slash GraphQL is fully managed GraphQL platform and the fastest way to build GraphQL apps.

Building an app with Slash GraphQL means you can use GraphQL natively and don't have to build other tech stacks or work out how to translate from some other technology to GraphQL. You work only with GraphQL and, think in terms of the graph that matters for your app.

That's the process we'll walk through together in this tutorial, starting on this page by looking at what we are going to build and how to build such an app in Slash GraphQL.  Then, the tutorial moves on to schema design with GraphQL, implementing a UI, authentication and authorization, subscriptions and then advanced topics like custom logic.

We'll be using the free tier on Slash GraphQL, you'll set up an account, but don't need to provide any payment details to complete the tutorial.

## What are we going to build

This tutorial will walk you through building a reasonably complete message board app.  There'll be lists of posts in different categories.

![App main page](/images/graphql/tutorial/discuss/main-screenshot.png)

We'll use Slash GraphQL's built in authorization to allow for public posts that anyone can see, even without logging in, but restrict some categories to be private unless you've been given permissions to view.  Logged in users can make new posts and each post can have a stream of comments, so other logged in users can comment on the messages.

![App post page](/images/graphql/tutorial/discuss/post-screenshot.png)

It's not the most original app every built, but there's enough in here to teach you about:

* Slash GraphQL
* Schema design with GraphQL
* GraphQL queries, mutations and subscriptions
* Building a UI with GraphQL
* Authentication and authorization
* Custom logic

We'll make this a completely serverless app for this tutorial and run the backend with Slash GraphQL, serverless authentication with Auth0 and deploy the frontend with Netlify.  I've been and done this already and you can use my version at https://slash-graphql-discuss.netlify.app/.

## Why GraphQL

You can build an app in any number of technologies, so why is GraphQL a good choice for this app?

GraphQL turns out to be a good choice in many situations, but particularly where the app data is inherantly a graph, and where GraphQL queries mean we can cut down on the complexity of the UI code.

In this case we have both.  The data for the app is itself a graph --- it's about users and posts and comments and the links between them.  We'll naturally want to explore that data graph as we work with the app, so GraphQL makes a great choice.  Also, in rendering the UI, GraphQL removes some complexity for us.

If we built the app with REST APIs, for example, our clients (e.g. Web, and mobile) will have to deal programatically with getting all the data to render a page.  For example, to render a post, we are likely to need to access the `/post/{id}` endpoint to get the post itself, then the `/comment?postid={id}` endpoint to get the comments, then, iteratively for each comment, access the `/author/{id}` endpoint.  We'd have to collect the data from those endpoints and build a data structure to render the UI.  That needs different code in each version of the app and increases our engineering effort and bug surface area.  

With GraphQL, rendering a page is simpler.  We run a single GraphQL query that gets all the data for a post, its comments and the authors of those comments, and then simply layout the page from the returned JSON.

## Why Slash GraphQL

Slash GraphQL lets you build a GraphQL API for your app from nothing but GraphQL and it gets you to a running GraphQL API faster than any other tool.

Often a GraphQL API is layered over a REST API or over a document or relational database. So, in those cases, GraphQL sits over other data sources and issues many queries to translate the REST/relational data into something that looks like a graph.  There's a cognitive jump there because your app is about a graph, but you've got to design a relational schema and work out how that translates as a graph; you'll think about the app in terms of the graph, but always have to mentally translate back and forth between the relational and graph models. There are engineering challenges around the translation as well as the efficiency of the queries.

There's none of that with Slash GraphQL.

Slash GraphQL is part of Dgraph, which stores a graph - it's a database of nodes and edges. So it's efficient to store, query and traverse as a graph. Your data will get stored just like you design it in the schema, and the queries are a single graph query that does just what the GraphQL query says.

With Slash GraphQL you design your application in GraphQL. You design a set of GraphQL types that describes your requirements. Slash GraphQL takes those types, prepares graph storage for them and generates a GraphQL API with queries and mutations.

You design a graph, store a graph and query a graph. You think and design in terms of the graph that your app is based around.

## App arcitechture

We are going to build a serverless app here, ... Auth0, Netlify, Slash ... not the only choice for a Slash app, but a good one for lots of situations and for this tutorial.

**FIXME: design image in here about how the app will work**
**FIXME: maybe a couple of images, to pull things apart and make simpler, but has to show Slash GraphQL backend, Auth provider, JWTs, frontend UI, in netlify.**

## What's next

First, we'll deploy a running Slash GraphQL backend that will host our GraphQL API.  That'll at get us something running that we can use to build out our app.

Then we'll move on to the design process - it's graph-first, in fact, it's GraphQL-first. We'll design the GraphQL types that our app is based around, and ... we'll there's no and ... from that, you get a GraphQL API for those types; you just move on to building the app around it.

