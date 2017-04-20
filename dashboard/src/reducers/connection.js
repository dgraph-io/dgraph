import { UPDATE_CONNECTED_STATE } from '../actions/connection';

const defaultState = {
  connected: false
}

const connection = (state = defaultState, action) => {
  switch (action.type) {
    case UPDATE_CONNECTED_STATE:
      return {
        ...state,
        connected: action.nextState
      };
    default:
      return state;
  }
};

export default connection;
