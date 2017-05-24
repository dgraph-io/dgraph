export const INCREMENT_VISIT_COUNT = "user/INCREMENT_VISIT_COUNT";
export const ANSWERED_NPS_SURVEY = "user/ANSWERED_NPS_SURVEY";
export const UPDATE_PREFERRED_SESSION_TAB = "user/UPDATE_PREFERRED_SESSION_TAB";

export function incrementVisitCount() {
  return {
    type: INCREMENT_VISIT_COUNT
  };
}

export function answeredNPSSurvey() {
  return {
    type: ANSWERED_NPS_SURVEY
  };
}

export function updatePreferredSessionTab(tabName) {
  return {
    type: UPDATE_PREFERRED_SESSION_TAB,
    tabName
  };
}
