import { combineReducers } from "redux";
import queries from "./queries";
import response from "./response";

const rootReducer = combineReducers({
    queries,
    response,
});

export default rootReducer;
