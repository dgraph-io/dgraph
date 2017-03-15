const query = (
    state = {
        text: "",
    },
    action,
) => {
    switch (action.type) {
        case "SELECT_QUERY":
            return {
                text: action.text,
            };
        default:
            return state;
    }
};

export default query;
