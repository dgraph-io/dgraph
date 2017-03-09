const interaction = (
    state = {
        node: "{}",
        partial: false,
    },
    action,
) => {
    switch (action.type) {
        case "SELECT_NODE":
            return {
                ...state,
                node: action.node,
            };
        case "UPDATE_PARTIAL":
            return {
                ...state,
                partial: action.partial,
            };
        case "RESET_RESPONSE":
            return {
                node: "{}",
                partial: false,
            };
        default:
            return state;
    }
};

export default interaction;
