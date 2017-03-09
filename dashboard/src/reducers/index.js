import { combineReducers } from "redux";
import previousQueries from "./previousQueries";
import query from "./query";
import response from "./response";
import interaction from "./interaction";

const rootReducer = combineReducers({
    query,
    previousQueries,
    response,
    interaction,
});

export default rootReducer;
