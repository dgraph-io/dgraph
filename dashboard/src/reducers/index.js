import { combineReducers } from "redux";

import frames from "./frames";
import connection from "./connection";
import query from "./query";

const rootReducer = combineReducers({
  frames,
  connection,
  query
});

export default rootReducer;
