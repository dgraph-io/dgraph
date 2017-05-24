import {
  INCREMENT_VISIT_COUNT,
  ANSWERED_NPS_SURVEY,
  UPDATE_PREFERRED_SESSION_TAB
} from "../actions/user";

const defaultState = {
  visitCount: 0,
  NPSSurveyDone: false,
  preferredSessionTab: "graph"
};

export default (state = defaultState, action) => {
  switch (action.type) {
    case INCREMENT_VISIT_COUNT:
      return {
        ...state,
        visitCount: state.visitCount + 1
      };
    case ANSWERED_NPS_SURVEY:
      return {
        ...state,
        NPSSurveyDone: true
      };
    case UPDATE_PREFERRED_SESSION_TAB:
      return {
        ...state,
        preferredSessionTab: action.tabName
      };
    default:
      return state;
  }
};
