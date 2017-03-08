import { REHYDRATE } from "redux-persist/constants";

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
            return [...state, query(undefined, action)];
        case "DELETE_QUERY":
            return [...state[action.idx], ...state[action.idx]];
        // case REHYDRATE:
        //     var incoming = action.payload.myReducer;
        //     console.log(action);
        //     if (incoming)
        //         return {
        //             ...state,
        //             ...incoming,
        //         };
        //     return state;
        default:
            return state;
    }
};

export default queries;
