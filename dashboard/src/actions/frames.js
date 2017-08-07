export const RECEIVE_FRAME = "frames/RECEIVE_FRAME";
export const DISCARD_FRAME = "frames/DISCARD_FRAME";
export const DISCARD_ALL_FRAMES = "frames/DISCARD_ALL_FRAMES";
export const UPDATE_FRAME = "frames/UPDATE_FRAME";

export function receiveFrame({ id, type, meta, query, share }) {
  return {
    type: RECEIVE_FRAME,
    frame: {
      id,
      type,
      meta,
      query,
      share
    }
  };
}

export function discardFrame(frameID) {
  return {
    type: DISCARD_FRAME,
    frameID
  };
}

export function discardAllFrames() {
  return {
    type: DISCARD_ALL_FRAMES
  };
}

// IDEA: the schema for frame object is getting complex. maybe use class optionally
// with flow?
export function updateFrame({ id, type, meta, data, query }) {
  return {
    type: UPDATE_FRAME,
    id,
    frame: {
      type,
      meta: meta || {}, // Default argument for meta
      query
    }
  };
}

// helpers

/**
 * toggleCollapseFrame returns an action object that will change the `collapsed`
 * state of a frame.
 *
 * @params frame {Object} - target frame
 * @params [nextState] {Boolean} - optional param to dictate if the frame should
 *     collapse. If not provided, the action will toggle the collapsed state
 */
export function toggleCollapseFrame(frame, nextState) {
  let shouldCollapse;
  if (nextState) {
    shouldCollapse = nextState;
  } else {
    shouldCollapse = !frame.meta.collapsed;
  }

  return updateFrame({
    id: frame.id,
    type: frame.type,
    meta: Object.assign({}, frame.meta, { collapsed: shouldCollapse }),
    query: frame.query
  });
}
