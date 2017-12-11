import {
  UPDATE_QUERY,
  UPDATE_ACTION,
  UPDATE_QUERY_AND_ACTION
} from "../actions/query";

const defaultState = {
  query: "",
  action: "query"
};

const frames = (state = defaultState, action) => {
  switch (action.type) {
    case UPDATE_QUERY:
      return {
        ...state,
        query: action.query
      };

    case UPDATE_ACTION:
      return {
        ...state,
        action: action.action
      };
    case UPDATE_QUERY_AND_ACTION:
      return {
        ...state,
        query: action.query,
        action: action.action
      };
    default:
      return state;
  }
};

export default frames;
