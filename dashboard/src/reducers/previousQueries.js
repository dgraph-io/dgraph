const query = (state, action) => {
    switch (action.type) {
        case "ADD_QUERY":
            return {
                text: action.text,
                lastRun: Date.now(),
            };
        default:
            return state;
    }
};

const queries = (state = [], action) => {
    switch (action.type) {
        case "ADD_QUERY":
            let trimmedQuery = action.text.trim();
            return [
                query(undefined, action),
                ...state.filter(q => q.text.trim() !== trimmedQuery),
            ];
        case "DELETE_QUERY":
            return [
                ...state.slice(0, action.idx),
                ...state.slice(action.idx + 1),
            ];
        default:
            return state;
    }
};

export default queries;
