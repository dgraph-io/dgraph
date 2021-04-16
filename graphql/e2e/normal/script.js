const authorBio = ({parent: {name, dob}}) => `My name is ${name} and I was born on ${dob}.`
const characterBio = ({parent: {name}}) => `My name is ${name}.`
const humanBio = ({parent: {name, totalCredits}}) => `My name is ${name}. I have ${totalCredits} credits.`
const droidBio = ({parent: {name, primaryFunction}}) => `My name is ${name}. My primary function is ${primaryFunction}.`
const summary = () => `hi`
const astronautBio = ({parent: {name, age, isActive}}) => `Name - ${name}, Age - ${age}, isActive - ${isActive}`

async function authorsByName({args, dql}) {
    const results = await dql.query(`query queryAuthor($name: string) {
        queryAuthor(func: type(Author)) @filter(eq(Author.name, $name)) {
            name: Author.name
            dob: Author.dob
            reputation: Author.reputation
        }
    }`, {"$name": args.name})
    return results.data.queryAuthor
}

async function newAuthor({args, graphql}) {
    // lets give every new author a reputation of 3 by default
    const results = await graphql(`mutation ($name: String!) {
        addAuthor(input: [{name: $name, reputation: 3.0 }]) {
            author {
                id
                reputation
            }
        }
    }`, {"name": args.name})
    return results.data.addAuthor.author[0].id
}

self.addGraphQLResolvers({
    "Author.bio": authorBio,
    "Character.bio": characterBio,
    "Human.bio": humanBio,
    "Droid.bio": droidBio,
    "Book.summary": summary,
    "Astronaut.bio": astronautBio,
    "Query.authorsByName": authorsByName,
    "Mutation.newAuthor": newAuthor
})

async function rank({parents}) {
    const idRepList = parents.map(function (parent) {
        return {id: parent.id, rep: parent.reputation}
    });
    const idRepMap = {};
    idRepList.sort((a, b) => a.rep > b.rep ? -1 : 1)
        .forEach((a, i) => idRepMap[a.id] = i + 1)
    return parents.map(p => idRepMap[p.id])
}

self.addMultiParentGraphQLResolvers({
    "Author.rank": rank
})

// TODO(GRAPHQL-1123): need to find a way to make it work on TeamCity machines.
// The host `172.17.0.1` used to connect to host machine from within docker, doesn't seem to
// work in teamcity machines, neither does `host.docker.internal` works there. So, we are
// skipping the related test for now.
async function districtWebhook({ dql, graphql, authHeader, event }) {
    // forward the event to the changelog server running on the host machine
    await fetch(`http://172.17.0.1:8888/changelog`, {
        method: "POST",
        body: JSON.stringify(event)
    })
    // just return, nothing else to do with response
}

self.addWebHookResolvers({
    "District.add": districtWebhook,
    "District.update": districtWebhook,
    "District.delete": districtWebhook,
})

