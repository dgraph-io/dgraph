const query = (
    state = {
        text: "",
        // Regex to match property name to display in Graph for nodes.
        propertyRegex: ""
    },
    action
) => {
    switch (action.type) {
        case "SELECT_QUERY":
            return {
                ...state,
                text: action.text
            };
        case "UPDATE_PROPERTY_REGEX":
            return {
                ...state,
                propertyRegex: action.regex
            };
        default:
            return state;
    }
};

export default query;
