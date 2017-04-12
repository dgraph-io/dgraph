const share = (
    state = {
        allowed: false,
        id: "",
        found: true
    },
    action
) => {
    switch (action.type) {
        case "UPDATE_SHARE_ID":
            return {
                ...state,
                id: action.shareId
            };
        case "QUERY_FOUND":
            return {
                ...state,
                found: action.found
            };
        case "UPDATE_ALLOWED":
            return {
                ...state,
                allowed: action.allowed
            };
        default:
            return state;
    }
};

export default share;
