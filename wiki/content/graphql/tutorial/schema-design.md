+++
title = "Schema Design"
[menu.main]
    parent = "build-an-app-tutorial"
    identifier = "schema-design"
    weight = 3   
+++

*In this section: we'll start designing the schema of our app and look at how graph schemas and graph queries work.*

To design the schema for our app we won't be thinking about tables or joins or documents, we'll be thinking about the entities in our app and how they are linked to make a graph.  Any requirements or design analysis needs iteration and thinking from a number of perspectives, so we'll work through some of that process and sketch out where we are going.

Graphs tend to model domains like our app really nicely because they naturally model things like the subgraph of a user and their posts and the comments on those posts, or the network of friends of a user, or the kinds of posts a user tends to like, so we'll look at how those kinds of graph queries work.

## UI requirements

Most apps are more than what you can see on the screen, but that's what we are focusing on here, and thinking about that will help kick off our design process, so let's at least start by looking at where we are going for a page and see what's there.

Although a single GraphQL query can save you lots of calls and return you a subgraph of data, a complete page might be built up of blocks that have different data requirements.  For example, in a sketch of our app's UI we can already see that forming.

![App UI requirements](/images/graphql/tutorial/discuss/UI-components.gif)

You can start to see the building blocks of the UI and some of the entities, like users, categories and posts that will form the data in our app.

## Thinking in Graphs

Designing a graph schema is about designing the things, or entities, that will form nodes in the graph, and designing the shape of the graph, or what links those entities have to other entities.

There's really two concepts in play here.  One is the data itself, often called the application data graph.  The other is the schema, which is itself graph shaped but really forms the pattern for the data graph.  You can think of the difference as somewhat similar to objects (or data structure definitions) verses instances in a program or a relation database schema verses rows of actual data.

Already we can start to tease out what some of the types of data and relationships in our graph are.  There's users who post posts, so we know there's a relationship between users and the posts they've made.  We know the posts are going to be assigned to some set of categories and that each post might have a stream of comments posted by users.

So our schema is going to have these kinds of entities and and relationships between them.

![Graph schema sketch](/images/graphql/tutorial/discuss/schema-inital-sketch.png)

I've borrowed some notation from other data modelling patterns here.  That's pretty much the modelling capability GraphQL will allow us, so let's start sketching with it for now.

A user is going to have some number of (zero or more `0..*`) posts and that a post can have exactly one author.  A post can be in only a single category, which, in turn, can contain many posts.  How does that translate into the application data graph?  Let's sketch out some examples.

Let's start with a single user who's posted three posts into a couple of different categories.  Our graph might start looking like this.

![Graph schema sketch](/images/graphql/tutorial/discuss/first-posts-in-graph.png)

Then another user joins and makes some posts.  Our graph gets a bit bigger and more interesting, but the types of things in the graph and the links they can have follow what the schema sets out as the pattern --- for example we aren't linking users to categories.

![Graph schema sketch](/images/graphql/tutorial/discuss/user2-posts-in-graph.png)

Next the users read some posts and start making and replying to comments.

![Graph schema sketch](/images/graphql/tutorial/discuss/comments-in-graph.png)

Each node in the graph will have the data (a bit like a document) that the schema says it can have, maybe a username for users and title, text and date published for posts, and the links to other nodes (the shape of the graph) as per what the schema allows.  

While we are still sketching things out here let's take a look at how queries will work.

## How graph queries work

Graph queries in GraphQL are really about entry points and traversals.  A query picks out, via a search, some nodes as a starting point and then selects data from the nodes or follows edges to traverse to other nodes.

For example, to render a user's information, we might need only to find the user.  So our use of the graph might be like in the following sketch --- we'll find the user as an entry point into the graph, perhaps from searching users by username, but not traverse any further, 

![Graph schema sketch](/images/graphql/tutorial/discuss/user1-search-in-graph.png)

Often, though, even in just presenting a user's information, we need to present information like most recent activity or sum up interest in recent posts.  So it's more likely that we'll start by finding the user as an entry point and then traversing some edges in the graph to explore a subgraph of interesting data.

![Graph schema sketch](/images/graphql/tutorial/discuss/user1-post-search-in-graph.png)

You can really start to see that traversal when it comes to rendering an individual post.  We'll need to find the post, probably by it's id when a user navigates to a url like `/post/0x2`, then we'll follow edges to the post's author and category, but we'll also need to follow the edges to all the comments, and from there to the authors of the comments.

![Graph schema sketch](/images/graphql/tutorial/discuss/post2-search-in-graph.png)

Graphs make these kinds of data traversals really clear, as compared to table joins or navigating your way through a RESTful API.  It can also really help to jot down a quick sketch.

It's also possible for a query to have multiple entry points and traversals from all of those entry points.  Imagine for example the query that renders the post list on the main page.  That's a query that finds multiple posts, maybe ordered by date or from particular categories, and then, for each, traverses to the author, category, etc.

We can now begin to imagine how our graphql queries to fill out the UI are going to work.  For example, in the sketch at the top, there will be a query starting at the logged in user to find their details, a query finding all the category nodes to fill out the category dropdown, and a more complex query that will find a number of posts and make traversals to find the posts' authors and categories.

## Schema

From our investigation about and thinking about what we are going to show for posts and users, we can start to flesh out our schema some more.

Posts for example are going to need a title and some text for the post, both string valued.  Posts will also need some sort of date to record when they were uploaded.  They'll also need links to the author, category and a list of comments.

As a next iteration, I've sketched out this much.

![Graph schema sketch](/images/graphql/tutorial/discuss/schema-sketch.png)

That's our first cut at a schema, or at the pattern our application data graph will follow.

We'll keep iterating on this as we work through the tutorial, that's what you'd do in building an app, no use pretending like we have all the answers at the start.  Eventually, we'll want to add likes and dislikes on the posts, maybe also tags, we'll also layer in a permissions system so some categories will require permissions to view.  But those are for later sections in the tutorial. This is enough to start building with.

## What's next

Next we'll make our design concrete by writing it down as a GraphQL schema and uploading that to Slash GraphQL.  That'll give us a running GraphQL API and we'll look at the queries and mutations that will form the data of our app.