import { UPDATE_QUERY } from "../actions/query";

const defaultState = {
  query: ""
};

const frames = (state = defaultState, action) => {
  switch (action.type) {
    case UPDATE_QUERY:
      return {
        ...state,
        query: action.query
      };
    default:
      return state;
  }
};

export default frames;
