import React from 'react';
import { invariant } from 'ts-invariant';
import { getApolloContext } from './ApolloContext';
export var ApolloConsumer = function (props) {
    var ApolloContext = getApolloContext();
    return (React.createElement(ApolloContext.Consumer, null, function (context) {
        invariant(context && context.client, 'Could not find "client" in the context of ApolloConsumer. ' +
            'Wrap the root component in an <ApolloProvider>.');
        return props.children(context.client);
    }));
};
//# sourceMappingURL=ApolloConsumer.js.map