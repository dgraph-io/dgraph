const interaction = (
    state = {
        node: "{}",
        partial: false,
        fullscreen: false,
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
        case "UPDATE_FULLSCREEN":
            return {
                ...state,
                fullscreen: action.fs,
            };
        default:
            return state;
    }
};

export default interaction;
