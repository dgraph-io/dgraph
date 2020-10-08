+++
title = "Overview"
weight = 1
[menu.main]
    parent = "authorization"
    identifier = "authorization-overview"
+++

Dgraph GraphQL comes with inbuilt authorization.  It allows you to annotate your schema with rules that determine who can access or mutate what data.

Firstly, let's get some concepts defined.  There are two important concepts in what's often called 'auth'.

* authentication : who are you; and
* authorization : what are you allowed to do.

Dgraph GraphQL deals with authorization, but is completely flexible about how your app does authentication.  You could authenticate your users with a cloud service like OneGraph or Auth0, use some social sign in options, or write bespoke code.  The connection between Dgraph and your authentication mechanism is a signed JWT - you tell Dgraph, for example, the public key of the JWT signer and Dgraph trusts JWTs signed by the corresponding private key.

With an authentication mechanism set up, you then annotate your schema with the `@auth` directive to define your authorization rules, attach details of your authentication provider to the last line of the schema, and pass the schema to Dgraph.  So your schema will follow this pattern.

```graphql
type A @auth(...) {
    ...
}

type B @auth(...) {
    ...
}

# Dgraph.Authorization {"VerificationKey":"","Header":"","Namespace":"","Algo":"","Audience":[]}
```

* `Header` is the header in which requests will send the signed JWT
* `Namespace` is the key inside the JWT that contains the claims relevant to Dgraph auth
* `Algo` is JWT verification algorithm which can be either `HS256` or `RS256`, and
* `VerificationKey` is the string value of the key (newlines replaced with `\n`) wrapped in `""`
* `Audience` is used to verify `aud` field of JWT which might be set by certain providers. It indicates the intended audience for the JWT. This is an optional field.

Valid examples look like

`# Dgraph.Authorization {"VerificationKey":"verificationkey","Header":"X-My-App-Auth","Namespace":"https://my.app.io/jwt/claims","Algo":"HS256","Audience":["aud1","aud5"]}`

Without audience field

`# Dgraph.Authorization {"VerificationKey":"secretkey","Header":"X-My-App-Auth","Namespace":"https://my.app.io/jwt/claims","Algo":"HS256"}`

for HMAC-SHA256 JWT with symmetric cryptography (the signing key and verification key are the same), and like

`# Dgraph.Authorization {"VerificationKey":"-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----","Header":"X-My-App-Auth","Namespace":"https://my.app.io/jwt/claims","Algo":"RS256"}`

for RSA Signature with SHA-256 asymmetric cryptography (the JWT is signed with the private key and Dgraph checks with the public key).

Both cases expect the JWT to be in a header `X-My-App-Auth` and expect the JWT to contain custom claims object `"https://my.app.io/jwt/claims": { ... }` with the claims used in authorization rules.

Note: authorization is in beta and some aspects may change - for example, it's possible that the method to specify the header, key, etc. will move into the /admin `updateGQLSchema` mutation that sets the schema.  Some features are also in active improvement and development - for example, auth is supported an on types, but interfaces (and the types that implement them) don't correctly support auth in the current beta.

---
