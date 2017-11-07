import SHA256 from "crypto-js/sha256";

import { checkStatus, getEndpoint, makeFrame } from "../lib/helpers";
import { FRAME_TYPE_LOADING } from "../lib/const";

import { receiveFrame } from "./frames";

/**
 * runQuery runs the query and displays the appropriate result in a frame
 * @params query {String}
 * @params [frameId] {String}
 *
 */
export const runQuery = query => {
  return dispatch => {
    const frame = makeFrame({ query });

    dispatch(receiveFrame(frame));
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
      uid
      _share_
  }
}`;

  return fetch(getEndpoint("query"), {
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
      const matchingQueries = result.data.query;

      // If no match, store the query
      if (matchingQueries.length === 0) {
        return createShare(queryText);
      }

      if (matchingQueries.length === 1) {
        return matchingQueries[0].uid;
      }

      // If more than one result, we have a hash collision. Break it.
      for (let i = 0; i < matchingQueries.length; i++) {
        const q = matchingQueries[i];
        if (`"${q._share_}"` === encodedQuery) {
          return q.uid;
        }
      }
    });
};

// runQueryByShareId runs the query by the given shareId and displays the frame
export const runQueryByShareId = shareId => {
  return dispatch => {
    const frame = makeFrame({
      type: FRAME_TYPE_LOADING,
      share: shareId
    });
    dispatch(receiveFrame(frame));
  };
};
