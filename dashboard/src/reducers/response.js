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
    isFetching: false,
    mutation: false
};

const response = (state = emptyState, action) => {
    switch (action.type) {
        case "ERROR_RESPONSE":
            return {
                ...state,
                text: action.text,
                success: false,
                data: action.json
            };
        case "SUCCESS_RESPONSE":
            return {
                ...state,
                data: action.data,
                text: action.text || "",
                success: true,
                mutation: action.isMutation
            };
        case "RESPONSE_PROPERTIES":
            return {
                ...state,
                ...action,
                numNodes: action.allNodes && action.allNodes.length,
                numEdges: action.allEdges && action.allEdges.length
            };
        case "RESET_RESPONSE":
            return emptyState;
        case "IS_FETCHING": {
            return {
                ...state,
                isFetching: action.fetching
            };
        }
        default:
            return state;
    }
};

export default response;
