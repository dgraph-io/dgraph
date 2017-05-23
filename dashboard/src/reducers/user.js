import { INCREMENT_VISIT_COUNT, ANSWERED_NPS_SURVEY } from "../actions/user";

const defaultState = {
  visitCount: 0,
  NPSSurveyDone: false
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
    default:
      return state;
  }
};
