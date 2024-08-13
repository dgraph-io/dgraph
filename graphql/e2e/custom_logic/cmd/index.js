const graphql = require("graphql");

// build internal graphql schema.
const graphqlSchemaObj = graphql.buildSchema(process.argv[2]);

// introspect and print the introspection result to stdout.
graphql.graphql(graphqlSchemaObj, graphql.introspectionQuery).then((res) => {
    console.log(JSON.stringify(res))
}) 