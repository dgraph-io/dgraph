import { combineReducers } from "redux";

import frames from "./frames";
import connection from "./connection";
import user from "./user";

const rootReducer = combineReducers({
  frames,
  connection,
  user
});

export default rootReducer;
