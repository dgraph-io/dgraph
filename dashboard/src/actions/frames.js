export const RECEIVE_FRAME = 'frames/RECEIVE_FRAME';
export const DISCARD_FRAME = 'frames/DISCARD_FRAME';
export const UPDATE_FRAME = 'frames/UPDATE_FRAME';

export function receiveFrame({ id, type, meta, data }) {
  return {
    type: RECEIVE_FRAME,
    frame: {
      id,
      type,
      meta,
      data
    }
  }
}

export function discardFrame(frameID) {
  return {
    type: DISCARD_FRAME,
    frameID
  };
}

// IDEA: the schema for frame object is getting complext. maybe use class optionally
// with flow?
export function updateFrame({ id, type, meta, data }) {
  return {
    type: UPDATE_FRAME,
    id,
    frame: {
      type,
      meta: meta || {}, // Default argument for meta
      data
    }
  }
}
