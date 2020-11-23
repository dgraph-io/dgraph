+++
title = "Overview"
weight = 1
[menu.main]
    parent = "authorization"
    identifier = "authorization-overview"
+++

Dgraph GraphQL comes with built-in authorization. It allows you to annotate your schema with rules that determine who can access or mutate the data.

First, let's get some concepts defined. There are two important concepts in what's often called 'auth':

* authentication: who are you,
* authorization: what are you allowed to do.

### Authentication

Dgraph GraphQL deals with authorization, but is completely flexible about how your app does authentication. You could authenticate your users with a cloud service like OneGraph or Auth0, use some social sign in options, or write bespoke code.  

The connection between Dgraph and your authentication mechanism can be a JSON Web Key URL (JWK URL) or a signed JSON Web Token (JWT). For example, you tell Dgraph the public key of the JWT signer and Dgraph trusts JWTs signed by the corresponding private key.

#### `Dgraph.Authorization` parameters

To define the connection method, you must set the `#Dgraph.Authorization` object:

```json
{"Header":"", "Namespace":"", "Algo":"", "VerificationKey":"", "JWKURL":"", "Audience":[]}
```

* `Header` is the header in which requests will send the signed JWT
* `Namespace` is the key inside the JWT that contains the claims relevant to Dgraph auth
* `Algo` is the JWT verification algorithm which can be either `HS256` or `RS256`
* `VerificationKey` is the string value of the key (newlines replaced with `\n`) wrapped in `""`
* `JWKURL` is the URL for the JSON Web Key sets
* `Audience` is used to verify the `aud` field of a JWT which might be set by certain providers. It indicates the intended audience for the JWT. When doing authentication with `JWKURL`, this field is mandatory as Identity Providers share JWKs among multiple tenants

To set up the authentication connection method:

**JSON Web Token (JWT)**
- A (`VerificationKey`, `Algo`) pair must be provided. The server will verify the JWT against the provided `VerificationKey`.

**JSON Web Key URL (JWK URL)**
- `JWKURL` and `Audience` must provided. The server will fetch all the JWKs and verify the token against one of the JWK, based on the JWK's kind.

{{% notice "note" %}}
You can only define one method, either `JWKURL` or `(VerificationKey, Algo)`, but not both.
{{% /notice %}}

### Authorization

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

Valid `Dgraph.Authorization` examples look like:

```
# Dgraph.Authorization {"VerificationKey":"verificationkey","Header":"X-My-App-Auth","Namespace":"https://my.app.io/jwt/claims","Algo":"HS256","Audience":["aud1","aud5"]}
```

Without audience field

```
# Dgraph.Authorization {"VerificationKey":"secretkey","Header":"X-My-App-Auth","Namespace":"https://my.app.io/jwt/claims","Algo":"HS256"}
```

for HMAC-SHA256 JWT with symmetric cryptography (the signing key and verification key are the same), and like

```
# Dgraph.Authorization {"VerificationKey":"-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----","Header":"X-My-App-Auth","Namespace":"https://my.app.io/jwt/claims","Algo":"RS256"}
```

for RSA Signature with SHA-256 asymmetric cryptography (the JWT is signed with the private key and Dgraph checks with the public key).

Both cases expect the JWT to be in a header `X-My-App-Auth` and expect the JWT to contain custom claims object `"https://my.app.io/jwt/claims": { ... }` with the claims used in authorization rules.

{{% notice "note" %}}
Authorization is in beta and some aspects may change - for example, it's possible that the method to specify the `header`, `key`, etc. will move into the /admin `updateGQLSchema` mutation that sets the schema. Some features are also in active improvement and development.
{{% /notice %}}
