#!/bin/bash

curl --request POST \
  --url http://localhost:8080/admin/schema \
  --header 'Content-Type: application/x-www-form-urlencoded' \
  --data 'type User {
   id: ID!
   name: String!
   Capital: String @lambda
}'

curl --request POST \
  --url http://localhost:8080/admin \
  --header 'Content-Type: application/json' \
  --data '{"query":"mutation {\n    updateLambdaScript(input: {set: {script:\"IGNvbnN0IGNhcE5hbWUgPSAoeyBwYXJlbnQ6IHsgbmFtZSB9IH0pID0+IHtyZXR1cm4gbmFtZS50b1VwcGVyQ2FzZSgpfQoKIHNlbGYuYWRkR3JhcGhRTFJlc29sdmVycyh7CiAgICJVc2VyLkNhcGl0YWwiOiBjYXBOYW1lLAp9KQ==\"}}){\n    \n# const capName = ({ parent: { name } }) => {return name.toUpperCase()}\n\n# self.addGraphQLResolvers({\n#   \"User.Capital\": capName,\n# })\n    \n    lambdaScript{\n      script\n    }\n  }\n}"}'

curl --request POST \
  --url http://localhost:8080/graphql \
  --header 'Content-Type: application/json' \
  --data '{"query":"mutation {\n  addUser(input:{name:\"Naman\"}) {\n    user {\n      name\n      Capital\n    }\n  }\n}\n"}'
