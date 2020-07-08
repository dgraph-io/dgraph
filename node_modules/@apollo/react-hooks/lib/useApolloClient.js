import React from 'react';
import { invariant } from 'ts-invariant';
import { getApolloContext } from '@apollo/react-common';
export function useApolloClient() {
    var client = React.useContext(getApolloContext()).client;
    invariant(client, 'No Apollo Client instance can be found. Please ensure that you ' +
        'have called `ApolloProvider` higher up in your tree.');
    return client;
}
//# sourceMappingURL=useApolloClient.js.map