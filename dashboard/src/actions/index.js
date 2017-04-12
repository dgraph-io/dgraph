import {
    timeout,
    checkStatus,
    isNotEmpty,
    showTreeView,
    processGraph,
    dgraphAddress,
    dgraphQuery
} from "../containers/Helpers";

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
        timeout(
            60000,
            fetch(dgraphAddress() + "/query?debug=true", {
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
        )
            .catch(function(error) {
                console.log(error.stack);
                var err = (error.response && error.response.text()) ||
                    error.message;
                return err;
            })
            .then(function(errorMsg) {
                if (errorMsg !== undefined) {
                    dispatch(fetchedResponse());
                    dispatch(saveErrorResponse(errorMsg));
                }
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

const doShareMutation = (dispatch, getState) => {
    let query = getState().query.text,
        stringifiedQuery = JSON.stringify(encodeURI(query)),
        mutation = `
    mutation {
        set {
            <_:share> <internal_share> ${stringifiedQuery} .
        }
    }`;

    fetch(dgraphQuery(false), {
        method: "POST",
        mode: "cors",
        headers: {
            Accept: "application/json",
            "Content-Type": "text/plain"
        },
        body: mutation
    })
        .then(checkStatus)
        .then(response => response.json())
        .then(function handleResponse(result) {
            if (result.uids && result.uids.share) {
                dispatch(updateShareId(result.uids.share));
            }
        })
        .catch(function(error) {
            console.log(error.stack);
            return error;
        })
        .then(function(errorMsg) {
            if (errorMsg !== undefined) {
                dispatch(
                    saveErrorResponse(
                        "Got error while saving querying for share: " + errorMsg
                    )
                );
            }
        });
};

export const getShareId = (dispatch, getState) => {
    let query = getState().query.text;
    if (query === "") {
        return;
    }
    let stringifiedQuery = JSON.stringify(encodeURI(query)),
        // Considering that index is already set on the pred, schema mutation
        // should be a no-op. Lets see if we have already stored this query by
        // performing an exact match.
        checkQuery = `
mutation {
    schema {
        internal_share: string @index(exact) .
    }
}
{
    query(func:eq(internal_share, ${stringifiedQuery})) {
        _uid_
    }
}`;

    timeout(
        6000,
        fetch(dgraphQuery(false), {
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
            .then(function handleResponse(result) {
                if (result.query && result.query.length > 0) {
                    return dispatch(updateShareId(result.query[0]["_uid_"]));
                } else {
                    // Else do a mutation to store the query.
                    return doShareMutation(dispatch, getState);
                }
            })
    )
        .catch(function(error) {
            console.log(error.stack);
            return error.message;
        })
        .then(function(errorMsg) {
            if (errorMsg !== undefined) {
                dispatch(
                    saveErrorResponse(
                        "Got error while saving querying for share: " + errorMsg
                    )
                );
            }
        });
};

export const getQuery = shareId => {
    return dispatch => {
        timeout(
            6000,
            fetch(dgraphQuery(false), {
                method: "POST",
                mode: "cors",
                headers: {
                    Accept: "application/json"
                },
                body: `{
                    query(id: ${shareId}) {
                        internal_share
                    }
                }`
            })
                .then(checkStatus)
                .then(response => response.json())
                .then(function handleResponse(result) {
                    if (result.query && result.query.length > 0) {
                        dispatch(
                            selectQuery(
                                decodeURI(result.query[0].internal_share)
                            )
                        );
                        return;
                    }
                    dispatch(queryFound(false));
                })
        )
            .catch(function(error) {
                return error.message;
            })
            .then(function(errorMsg) {
                if (errorMsg !== undefined) {
                    dispatch(
                        saveErrorResponse(
                            `Got error while getting query for id: ${shareId}, err: ` +
                                errorMsg
                        )
                    );
                }
            });
    };
};

const updateAllowed = allowed => ({
    type: "UPDATE_ALLOWED",
    allowed: allowed
});

export const initialServerState = () => {
    return dispatch => {
        timeout(
            6000,
            fetch([dgraphAddress(), "ui/init"].join("/"), {
                method: "GET",
                mode: "cors",
                headers: {
                    Accept: "application/json"
                }
            })
                .then(checkStatus)
                .then(response => response.json())
                .then(function handleResponse(result) {
                    dispatch(updateAllowed(result.share));
                })
        )
            .catch(function(error) {
                console.log(error.stack);
                return error.message;
            })
            .then(function(errorMsg) {
                if (errorMsg !== undefined) {
                    dispatch(
                        saveErrorResponse(
                            "Got error while communicating with server: " +
                                errorMsg
                        )
                    );
                }
            });
    };
};
