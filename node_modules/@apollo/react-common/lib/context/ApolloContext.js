import React from 'react';
var apolloContext;
export function getApolloContext() {
    if (!apolloContext) {
        apolloContext = React.createContext({});
    }
    return apolloContext;
}
export function resetApolloContext() {
    apolloContext = React.createContext({});
}
//# sourceMappingURL=ApolloContext.js.map