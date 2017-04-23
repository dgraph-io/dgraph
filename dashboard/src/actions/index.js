import {
    timeout,
    checkStatus,
    isNotEmpty,
    showTreeView,
    processGraph,
    getEndpoint
} from "../containers/Helpers";
import SHA256 from "crypto-js/sha256";

// TODO - Check if its better to break this file down into multiple files.

export const updatePartial = partial => ({
    type: "UPDATE_PARTIAL",
    partial
});

export const selectQuery = text => ({
    type: "SELECT_QUERY",
    text
});

export const deleteQuery = idx => ({
    type: "DELETE_QUERY",
    idx
});

export const deleteAllQueries = () => ({
    type: "DELETE_ALL_QUERIES"
});

export const setCurrentNode = node => ({
    type: "SELECT_NODE",
    node
});

const addQuery = text => ({
    type: "ADD_QUERY",
    text
});

const isFetching = () => ({
    type: "IS_FETCHING",
    fetching: true
});

const fetchedResponse = () => ({
    type: "IS_FETCHING",
    fetching: false
});

const saveSuccessResponse = (text, data, isMutation) => ({
    type: "SUCCESS_RESPONSE",
    text,
    data,
    isMutation
});

const saveErrorResponse = (text, json = {}) => ({
    type: "ERROR_RESPONSE",
    text,
    json
});

const saveResponseProperties = obj => ({
    type: "RESPONSE_PROPERTIES",
    ...obj
});

export const updateLatency = obj => ({
    type: "UPDATE_LATENCY",
    ...obj
});

export const renderGraph = (query, result, treeView) => {
    return (dispatch, getState) => {
        let [nodes, edges, labels, nodesIdx, edgesIdx] = processGraph(
            result,
            treeView,
            query,
            getState().query.propertyRegex
        );

        dispatch(
            updateLatency({
                server: result.server_latency && result.server_latency.total
            })
        );

        dispatch(updatePartial(nodesIdx < nodes.length));

        dispatch(
            saveResponseProperties({
                plotAxis: labels,
                allNodes: nodes,
                allEdges: edges,
                numNodes: nodes.length,
                numEdges: edges.length,
                nodes: nodes.slice(0, nodesIdx),
                edges: edges.slice(0, edgesIdx),
                treeView: treeView,
                data: result
            })
        );
    };
};

export const resetResponseState = () => ({
    type: "RESET_RESPONSE"
});

export const runQuery = query => {
    return dispatch => {
        dispatch(resetResponseState());
        dispatch(isFetching());

        return timeout(
            60000,
            fetch(getEndpoint('query', { debug: true }), {
                method: "POST",
                mode: "cors",
                headers: {
                    "Content-Type": "text/plain"
                },
                body: query
            })
                .then(checkStatus)
                .then(response => response.json())
                .then(function handleResponse(result) {
                    dispatch(fetchedResponse());
                    if (
                        result.code !== undefined &&
                        result.message !== undefined
                    ) {
                        if (result.code.startsWith("Error")) {
                            dispatch(saveErrorResponse(result.message));
                            // This is the case in which user sends a mutation.
                            // We display the response from server.
                        } else {
                            dispatch(addQuery(query));
                            dispatch(saveSuccessResponse("", result, true));
                        }
                    } else if (isNotEmpty(result)) {
                        dispatch(addQuery(query));
                        let mantainSortOrder = showTreeView(query);
                        dispatch(saveSuccessResponse("", result, false));
                        dispatch(renderGraph(query, result, mantainSortOrder));
                    } else {
                        dispatch(
                            saveErrorResponse(
                                "Your query did not return any results.",
                                result
                            )
                        );
                    }
                })
        ).catch(function(error) {
            dispatch(fetchedResponse());
            dispatch(saveErrorResponse(error.message));
        });
    };
};

export const updateFullscreen = fs => ({
    type: "UPDATE_FULLSCREEN",
    fs
});

export const updateProgress = perc => ({
    type: "UPDATE_PROGRESS",
    perc,
    display: true
});

export const hideProgressBar = () => ({
    type: "HIDE_PROGRESS",
    dispatch: false
});

export const updateRegex = regex => ({
    type: "UPDATE_PROPERTY_REGEX",
    regex
});

export const addScratchpadEntry = entry => ({
    type: "ADD_SCRATCHPAD_ENTRY",
    ...entry
});

export const deleteScratchpadEntries = () => ({
    type: "DELETE_SCRATCHPAD_ENTRIES"
});

export const updateShareId = shareId => ({
    type: "UPDATE_SHARE_ID",
    shareId
});

export const queryFound = found => ({
    type: "QUERY_FOUND",
    found: found
});

// createShare posts the query to the server to be persisted
const createShare = (dispatch, getState) => {
    const query = getState().query.text;
    const stringifiedQuery = encodeURI(query);

    fetch(getEndpoint('share'), {
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
        .then((result) => {
            if (result.uids && result.uids.share) {
                dispatch(updateShareId(result.uids.share));
            }
        })
        .catch(function(error) {
            dispatch(
                saveErrorResponse(
                    "Got error while saving querying for share: " +
                        error.message
                )
            );
        });
};

export const getShareId = (dispatch, getState) => {
    const query = getState().query.text;
    if (query === "") {
        return;
    }
    const encodedQuery = encodeURI(query);
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
        fetch(getEndpoint('query'), {
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
            .then((result) => {
                const matchingQueries = result.query;

                // If no match, store the query
                if (!matchingQueries) {
                    return createShare(dispatch, getState);
                }
                if (matchingQueries.length === 1) {
                    return dispatch(updateShareId(result.query[0]._uid_));
                } else if (matchingQueries.length > 1) {
                    for (let i = 0; i < matchingQueries.length; i++) {
                        const q = matchingQueries[i];
                        if (`"${q._share_}"` === encodedQuery) {
                            return dispatch(updateShareId(q._uid_));
                        }
                    }
                }
            })
    ).catch(function(error) {
        dispatch(
            saveErrorResponse(
                "Got error while saving querying for share: " + error.message
            )
        );
    });
};

export const getQuery = shareId => {
    return dispatch => {
        timeout(
            6000,
            fetch(getEndpoint('query'), {
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
                .then(checkStatus)
                .then(response => response.json())
                .then(function(result) {
                    if (result.query && result.query.length > 0) {
                        dispatch(selectQuery(decodeURI(result.query[0]._share_)));
                    } else {
                        dispatch(queryFound(false));
                    }
                })
        ).catch(function(error) {
            dispatch(
                saveErrorResponse(
                    `Got error while getting query for id: ${shareId}, err: ` +
                        error.message
                )
            );
        });
    };
};
