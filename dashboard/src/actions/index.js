import {
    timeout,
    checkStatus,
    parseJSON,
    isNotEmpty,
    showTreeView,
    processGraph,
} from "../containers/Helpers";

export const updatePartial = partial => ({
    type: "UPDATE_PARTIAL",
    partial,
});

export const selectQuery = text => ({
    type: "SELECT_QUERY",
    text,
});

export const deleteQuery = idx => ({
    type: "DELETE_QUERY",
    idx,
});

export const setCurrentNode = node => ({
    type: "SELECT_NODE",
    node,
});

const addQuery = text => ({
    type: "ADD_QUERY",
    text,
});

const isFetching = () => ({
    type: "IS_FETCHING",
    fetching: true,
});

const fetchedResponse = () => ({
    type: "IS_FETCHING",
    fetching: false,
});

const saveSuccessResponse = (text, data) => ({
    type: "SUCCESS_RESPONSE",
    text,
    data,
});

const saveErrorResponse = text => ({
    type: "ERROR_RESPONSE",
    text,
});

const saveResponseProperties = obj => ({
    type: "RESPONSE_PROPERTIES",
    ...obj,
});

export const renderGraph = (query, result, treeView) => {
    return dispatch => {
        let startTime = new Date();

        let [nodes, edges, labels, nodesIdx, edgesIdx] = processGraph(
            result,
            treeView,
            query,
        );

        let endTime = new Date(),
            timeTaken = (endTime.getTime() - startTime.getTime()) / 1000,
            render = "";

        if (timeTaken > 1) {
            render = timeTaken.toFixed(1) + "s";
        } else {
            render = (timeTaken - Math.floor(timeTaken)) * 1000 + "ms";
        }

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
                latency: result.server_latency.total,
                data: result,
                rendering: render,
            }),
        );
    };
};

export const resetResponseState = () => ({
    type: "RESET_RESPONSE",
});

export const runQuery = query => {
    return dispatch => {
        dispatch(isFetching());
        dispatch(resetResponseState);
        // TODO - Add timeout back

        timeout(
            60000,
            fetch(process.env.REACT_APP_DGRAPH + "/query?debug=true", {
                method: "POST",
                mode: "cors",
                headers: {
                    "Content-Type": "text/plain",
                },
                body: query,
            })
                .then(checkStatus)
                .then(parseJSON)
                .then(function handleResponse(result) {
                    dispatch(fetchedResponse());
                    // This is the case in which user sends a mutation. We display the response from server.
                    if (
                        result.code !== undefined &&
                        result.message !== undefined
                    ) {
                        dispatch(addQuery(query));
                        dispatch(saveSuccessResponse(JSON.stringify(result)));
                    } else if (isNotEmpty(result)) {
                        dispatch(addQuery(query));
                        let mantainSortOrder = showTreeView(query);
                        dispatch(saveSuccessResponse(null, result));
                        renderGraph(query, result, mantainSortOrder)(dispatch);
                    } else {
                        dispatch(
                            saveErrorResponse(
                                "Your query did not return any results.",
                            ),
                        );
                    }
                }),
        )
            .catch(function(error) {
                console.log(error.stack);
                var err = error.response && error.response.text() ||
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
    fs,
});
