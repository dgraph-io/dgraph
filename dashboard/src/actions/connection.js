export const UPDATE_CONNECTED_STATE = 'connection/UPDATE_CONNECTED_STATE';

import { timeout, getEndpoint } from "../lib/helpers";

export function updateConnectedState(nextState) {
  return {
    type: UPDATE_CONNECTED_STATE,
    nextState
  }
}

/**
 * refreshConnectedState checks if the query endpoint responds and updates the
 * connected state accordingly
 */
export function refreshConnectedState() {
  return (dispatch) => {
    return timeout(
      6000,
      fetch(getEndpoint('query'), {
        method: "POST",
        mode: "cors",
        headers: {
          Accept: "application/json"
        },
        body: `{}`
      })
    )
      .then((response) => {
        let nextConnectedState;
        if (response.status === 200) {
          nextConnectedState = true;
        } else {
          nextConnectedState = false;
        }

        dispatch(updateConnectedState(nextConnectedState));
      })
      .catch((e) => {
        console.log(e.stack);

        dispatch(updateConnectedState(false));
      })
  }
}
