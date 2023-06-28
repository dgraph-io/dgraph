 /*
 Test functions to use with following Schema
 type Query {
  dqlquery(query: String): String @lambda
  gqlquery(query: String): String @lambda
  dqlmutate(query: String): String @lambda
}

*/
const echo = async ({args, authHeader, graphql, dql,accessJWT}) => {
  let payload=JSON.parse(atob(accessJWT.split('.')[1]));
  return  `args: ${JSON.stringify(args)}
  accesJWT: ${accessJWT}
  authHeader: ${JSON.stringify(authHeader)}
  namespace: ${payload.namespace}`
}
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
    "Query.echo": echo,
  });

  async function mutate(dql,rdfs) {
  if (rdfs !== "") {
        return dql.mutate(`{
                set {
                    ${rdfs}
                }
            }`)
    }
}
  async function addInfo({event, dql, graphql, authHeader, accessJWT}) {

      var rdfs = "";
      for (let i = 0; i < event.add.rootUIDs.length; ++i ) {
          console.log(`webhook on uid ${event.add.rootUIDs[i]} text:${event.add.input[i]['text']}`)
          const uid = event.add.rootUIDs[i];
          rdfs += `<${uid}>  <message> "${accessJWT}" .
          `;
      }
      await mutate(dql,rdfs);
  }
  async function deleteInfo({event, dql, graphql, authHeader, accessJWT}) {

      var rdfs = "";
      for (let i = 0; i < event.add.rootUIDs.length; ++i ) {
          console.log(`webhook delete on uid ${event.add.rootUIDs[i]} text:${event.add.input[i]['text']}`)
      }
      await mutate(dql,rdfs);
  }
  self.addWebHookResolvers({
   "Info.add": addInfo,
   "Info.delete": deleteInfo
})
