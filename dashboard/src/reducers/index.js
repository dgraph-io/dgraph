import { combineReducers } from "redux";
import previousQueries from "./previousQueries";
import query from "./query";
import response from "./response";
import interaction from "./interaction";
import latency from "./latency";

const rootReducer = combineReducers({
    query,
    previousQueries,
    response,
    interaction,
    latency
});

export default rootReducer;
