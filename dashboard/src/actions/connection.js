export const UPDATE_CONNECTED_STATE = "connection/UPDATE_CONNECTED_STATE";

import { getEndpoint } from "../lib/helpers";

export function updateConnectedState(nextState) {
  return {
    type: UPDATE_CONNECTED_STATE,
    nextState
  };
}

/**
 * refreshConnectedState checks if the query endpoint responds and updates the
 * connected state accordingly
 */
export function refreshConnectedState() {
  return dispatch => {
    return fetch(getEndpoint("health"), {
      method: "GET",
      mode: "cors",
      headers: {
        Accept: "application/json"
      }
    })
      .then(response => {
        let nextConnectedState;
        if (response.status === 200) {
          nextConnectedState = true;
        } else {
          nextConnectedState = false;
        }

        dispatch(updateConnectedState(nextConnectedState));
      })
      .catch(e => {
        console.log(e.stack);

        dispatch(updateConnectedState(false));
      });
  };
}
