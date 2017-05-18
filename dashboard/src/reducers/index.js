import { combineReducers } from "redux";

import frames from "./frames";
import connection from "./connection";

const rootReducer = combineReducers({
  frames,
  connection
});

export default rootReducer;
