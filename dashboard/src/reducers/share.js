const share = (
    state = {
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
        default:
            return state;
    }
};

export default share;
