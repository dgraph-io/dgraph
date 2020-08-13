+++
title = "Documentation and Comments"
[menu.main]
    parent = "schema"
    weight = 6   
+++

More documentation about documentation comments coming soon :-)

Dgraph accepts GraphQL documentation comments `"""..."""`  that gets passed through to the generated API and thus shown as documentation in GraphQL tools like GraphiQL, GraphQL Playground, Insomnia etc.

You can also add `# ...` comments where ever you like.  Those are just like code comments in the input schema and get dropped.  

Any comment starting with `# Dgraph.` is reserved and shouldn't be used to document your input schema.
