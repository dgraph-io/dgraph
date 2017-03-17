import _ from "lodash/object";

const emptyState = {
    server: "",
    rendering: {
        start: undefined,
        end: undefined
    }
};

const latency = (state = emptyState, action) => {
    switch (action.type) {
        case "UPDATE_LATENCY":
            return _.merge({}, state, action);
        case "RESET_RESPONSE":
            return emptyState;
        default:
            return state;
    }
};

export default latency;
