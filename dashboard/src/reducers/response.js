const emptyState = {
    text: "",
    data: {},
    success: false,
    plotAxis: [],
    nodes: [],
    edges: [],
    allNodes: [],
    allEdges: [],
    numNodes: 0,
    numEdges: 0,
    treeView: false,
    latency: "",
    rendering: "",
    isFetching: false,
};

const response = (state = emptyState, action) => {
    switch (action.type) {
        case "ERROR_RESPONSE":
            return {
                ...state,
                text: action.text,
                success: false,
            };
        case "SUCCESS_RESPONSE":
            return {
                ...state,
                data: action.data,
                text: action.text || "",
                success: true,
            };
        case "RESPONSE_PROPERTIES":
            // TODO - Exclude type from action.
            return {
                ...state,
                ...action,
                numNodes: action.allNodes.length,
                numEdges: action.allEdges.length,
            };
        case "RESET_RESPONSE":
            return emptyState;
        case "IS_FETCHING": {
            return {
                ...state,
                isFetching: action.fetching,
            };
        }
        default:
            return state;
    }
};

export default response;
