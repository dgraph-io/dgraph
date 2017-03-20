const scratchpad = (state = [], action) => {
    switch (action.type) {
        case "ADD_SCRATCHPAD_ENTRY":
            let entry = {
                name: action.name,
                uid: action.uid
            };
            let idx = state.map(e => e.uid).indexOf(entry.uid);
            if (idx !== -1) {
                return state;
            }

            return [...state, entry];
        case "DELETE_SCRATCHPAD_ENTRIES":
            return [];
        default:
            return state;
    }
};

export default scratchpad;
