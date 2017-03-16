const emptyState = {
    node: "{}",
    partial: false,
    fullscreen: false,
    progress: {
        perc: 0,
        display: false
    }
};

const interaction = (state = emptyState, action) => {
    switch (action.type) {
        case "SELECT_NODE":
            return {
                ...state,
                node: action.node
            };
        case "UPDATE_PARTIAL":
            return {
                ...state,
                partial: action.partial
            };
        case "RESET_RESPONSE":
            return emptyState;
        case "UPDATE_FULLSCREEN":
            return {
                ...state,
                fullscreen: action.fs
            };
        case "UPDATE_PROGRESS":
            return {
                ...state,
                progress: {
                    perc: action.perc,
                    display: true
                }
            };
        case "HIDE_PROGRESS":
            return {
                ...state,
                progress: {
                    display: false
                }
            };
        default:
            return state;
    }
};

export default interaction;
