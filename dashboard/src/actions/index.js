import SHA256 from "crypto-js/sha256";
import Raven from "raven-js";

import {
  timeout,
  checkStatus,
  isNotEmpty,
  getEndpoint,
  makeFrame
} from "../lib/helpers";
import {
  FRAME_TYPE_SESSION,
  FRAME_TYPE_SUCCESS,
  FRAME_TYPE_LOADING,
  FRAME_TYPE_ERROR
} from "../lib/const";
import { processGraph } from "../lib/graph";

import { receiveFrame, updateFrame } from "./frames";
import { updateConnectedState } from "./connection";

// executeQueryAndUpdateFrame fetches the query response from the server
// and updates the frame
function executeQueryAndUpdateFrame(dispatch, { frameId, query }) {
  const endpoint = getEndpoint("query", { debug: true });

  return timeout(
    60000,
    fetch(endpoint, {
      method: "POST",
      mode: "cors",
      cache: "no-cache",
      headers: {
        "Content-Type": "text/plain"
      },
      body: query
    })
  )
    .then(checkStatus)
    .then(response => response.json())
    .then(result => {
      dispatch(updateConnectedState(true));

      if (result.code !== undefined && result.message !== undefined) {
        // This is the case in which user sends a mutation.
        // We display the response from server.
        let frameType;
        if (result.code.startsWith("Error")) {
          frameType = FRAME_TYPE_ERROR;
        } else {
          frameType = FRAME_TYPE_SUCCESS;
        }

        dispatch(
          updateFrame({
            id: frameId,
            type: frameType,
            data: {
              query,
              message: result.message,
              response: result
            }
          })
        );
      } else if (isNotEmpty(result)) {
        const { nodes, edges, labels, nodesIndex, edgesIndex } = processGraph(
          result,
          false,
          query
        );

        dispatch(
          updateFrame({
            id: frameId,
            type: FRAME_TYPE_SESSION,
            data: {
              query,
              response: {
                plotAxis: labels,
                allNodes: nodes,
                allEdges: edges,
                numNodes: nodes.length,
                numEdges: edges.length,
                nodes: nodes.slice(0, nodesIndex),
                edges: edges.slice(0, edgesIndex),
                treeView: false,
                data: result
              }
            }
          })
        );
      } else {
        dispatch(
          updateFrame({
            id: frameId,
            type: FRAME_TYPE_SUCCESS,
            data: {
              query,
              message: "Your query did not return any results",
              response: result
            }
          })
        );
      }
    })
    .catch(error => {
      // FIXME: make it DRY. but error.response.text() is async and error.message is sync

      // if no response, it's a network error or client side runtime error
      if (!error.response) {
        // Capture client side error not query execution error from server
        // FIXME: This captures 404
        Raven.captureException(error);

        dispatch(updateConnectedState(false));
        dispatch(
          updateFrame({
            id: frameId,
            type: FRAME_TYPE_ERROR,
            data: {
              query,
              message: `${error.message}: Could not connect to the server`,
              response: error
            }
          })
        );
      } else {
        error.response.text().then(text => {
          dispatch(
            updateFrame({
              id: frameId,
              type: FRAME_TYPE_ERROR,
              data: {
                query,
                message: text,
                response: error
              }
            })
          );
        });
      }
    });
}

/**
 * runQuery runs the query and displays the appropriate result in a frame
 * @params query {String}
 * @params [frameId] {String}
 *
 * If frameId is not given, It inserts a new frame. Otherwise, it updates the
 * frame with the id equal to frameId
 *
 */
export const runQuery = (query, frameId) => {
  return dispatch => {
    // Either insert a new frame or update
    let targetFrameId;
    if (frameId) {
      dispatch(
        updateFrame({
          id: frameId,
          type: FRAME_TYPE_LOADING,
          data: {}
        })
      );
      targetFrameId = frameId;
    } else {
      const frame = makeFrame({
        type: FRAME_TYPE_LOADING,
        data: {}
      });
      dispatch(receiveFrame(frame));
      targetFrameId = frame.id;
    }

    return executeQueryAndUpdateFrame(dispatch, {
      frameId: targetFrameId,
      query
    });
  };
};

export const addScratchpadEntry = entry => ({
  type: "ADD_SCRATCHPAD_ENTRY",
  ...entry
});

export const deleteScratchpadEntries = () => ({
  type: "DELETE_SCRATCHPAD_ENTRIES"
});

// createShare persists the queryText in the database
const createShare = queryText => {
  const stringifiedQuery = encodeURI(queryText);

  return fetch(getEndpoint("share"), {
    method: "POST",
    mode: "cors",
    headers: {
      Accept: "application/json",
      "Content-Type": "text/plain"
    },
    body: stringifiedQuery
  })
    .then(checkStatus)
    .then(response => response.json())
    .then(result => {
      if (result.uids && result.uids.share) {
        return result.uids.share;
      }
    });
};

/**
 * getShareId gets the id used to share a query either by fetching one if one
 * exists, or persisting the queryText into the database.
 *
 * @params queryText {String} - A raw query text as entered by the user
 * @returns {Promise}
 */
export const getShareId = queryText => {
  const encodedQuery = encodeURI(queryText);
  const queryHash = SHA256(encodedQuery).toString();
  const checkQuery = `
{
  query(func:eq(_share_hash_, ${queryHash})) {
      _uid_
      _share_
  }
}`;

  return timeout(
    6000,
    fetch(getEndpoint("query"), {
      method: "POST",
      mode: "cors",
      headers: {
        Accept: "application/json",
        "Content-Type": "text/plain"
      },
      body: checkQuery
    })
      .then(checkStatus)
      .then(response => response.json())
      .then(result => {
        const matchingQueries = result.query;

        // If no match, store the query
        if (!matchingQueries) {
          return createShare(queryText);
        }

        if (matchingQueries.length === 1) {
          return result.query[0]._uid_;
        }

        // If more than one result, we have a hash collision. Break it.
        for (let i = 0; i < matchingQueries.length; i++) {
          const q = matchingQueries[i];
          if (`"${q._share_}"` === encodedQuery) {
            return q._uid_;
          }
        }
      })
      .catch(error => {
        Raven.captureException(error);
      })
  );
};

/**
 * getSharedQuery returns a promise that resolves with the query string corresponding
 * to the given shareId. Concretely, it fetches from the database the query
 * stored with the shareId. If not found, the promise resolves with an empty string.
 *
 * @params shareId {String}
 * @returns {Promise}
 *
 */
export const getSharedQuery = shareId => {
  return timeout(
    6000,
    fetch(getEndpoint("query"), {
      method: "POST",
      mode: "cors",
      headers: {
        Accept: "application/json"
      },
      body: `{
          query(id: ${shareId}) {
              _share_
          }
      }`
    })
  )
    .then(checkStatus)
    .then(response => response.json())
    .then(function(result) {
      if (result.query && result.query.length > 0) {
        const query = decodeURI(result.query[0]._share_);
        return query;
      } else {
        return "";
      }
    })
    .catch(function(error) {
      Raven.captureException(error);

      console.log(
        `Got error while getting query for id: ${shareId}, err: ${error.message}`
      );
    });
};

// runQueryByShareId runs the query by the given shareId and displays the frame
export const runQueryByShareId = shareId => {
  return dispatch => {
    const frame = makeFrame({
      type: FRAME_TYPE_LOADING,
      data: {}
    });
    dispatch(receiveFrame(frame));

    return getSharedQuery(shareId)
      .then(query => {
        if (!query) {
          return dispatch(
            updateFrame({
              id: frame.id,
              type: FRAME_TYPE_ERROR,
              data: {
                query: "", // TOOD: make query optional
                message: `No query found for the shareId: ${shareId}`,
                response: JSON.stringify("{}") // TOOD: make response optional
              }
            })
          );
        }

        return executeQueryAndUpdateFrame(dispatch, {
          query,
          frameId: frame.id
        });
      })
      .catch(error => {
        Raven.captureException(error);

        return dispatch(
          updateFrame({
            id: frame.id,
            type: FRAME_TYPE_ERROR,
            data: {
              query: "",
              message: error.message,
              response: JSON.stringify(error)
            }
          })
        );
      });
  };
};
