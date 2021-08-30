const fullName = ({ parent: { firstName, lastName } }) =>
  `${firstName} ${lastName}`;

async function todoTitles({ graphql }) {
  const results = await graphql("{ queryTodo { title } }");
  return results.data.queryTodo.map((t) => t.title);
}

self.addGraphQLResolvers({
  "User.fullName": fullName,
  "Query.todoTitles": todoTitles,
});

async function reallyComplexDql({ parents, dql }) {
  const ids = parents.map((p) => p.id);
  const someComplexResults = await dql.query(
    `really-complex-query-here with ${ids}`
  );
  return parents.map((parent) => someComplexResults[parent.id]);
}

self.addMultiParentGraphQLResolvers({
  "User.reallyComplexProperty": reallyComplexDql,
});
