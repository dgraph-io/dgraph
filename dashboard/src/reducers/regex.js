import {REHYDRATE} from 'redux-persist/constants'

const regex = (
    state = {
        // Regex to match property name to display in Graph for nodes.
        propertyRegex: ""
    },
    action
) => {
    switch (action.type) {
        case "UPDATE_PROPERTY_REGEX":
            return {
                ...state,
                propertyRegex: action.regex
            };
        case REHYDRATE:
            var incoming = action.payload.regex
            if (!incoming || incoming.propertyRegex === "") {
                return {
                    propertyRegex: "name"
                }
            }
            if (incoming) {
                return incoming
            }
            return state
        default:
            return state;
    }
};

export default regex;
