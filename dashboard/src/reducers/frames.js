import { RECEIVE_FRAME, DISCARD_FRAME, UPDATE_FRAME } from '../actions/frames';

const defaultState = {
  items: []
}

const frames = (state = defaultState, action) => {
  switch (action.type) {
    case RECEIVE_FRAME:
      return {
        ...state,
        items: [ action.frame, ...state.items ]
      };
    case DISCARD_FRAME:
      return {
        ...state,
        items: state.items.filter(item => item.id !== action.frameID)
      }
    case UPDATE_FRAME:
      return {
        ...state,
        items: state.items.map(item => {
          if (item.id === action.id) {
            return { ...item, ...action.frame }
          }

          return item;
        })
      }
    default:
      return state;
  }
};

export default frames;
