 /*
 Test functions to use with following Schema
 type Query {
  dqlquery(query: String): String @lambda
  gqlquery(query: String): String @lambda
  dqlmutate(query: String): String @lambda
}

*/
 const dqlquery = async ({args, authHeader, graphql, dql}) => {
  
    const dqlQ = await dql.query(`${args.query}`)
    return  JSON.stringify(dqlQ)
  }
  const gqlquery = async ({args, authHeader, graphql, dql}) => {
    const gqlQ = await graphql(`${args.query}`)
    return  JSON.stringify(gqlQ)
  }
  const dqlmutation = async ({args, authHeader, graphql, dql}) => {
     // Mutate User with DQL
    const dqlM = await dql.mutate(`${args.query}`)
    return  JSON.stringify(dqlM)
  }
  
  self.addGraphQLResolvers({
    "Query.dqlquery": dqlquery,
    "Query.gqlquery": gqlquery,
    "Query.dqlmutate": dqlmutation,
  });
