const response = (
    state = {
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
    },
    action,
) => {
    switch (action.type) {
        case "ERROR_RESPONSE":
            return {
                ...state,
                text: action.text,
            };
        case "SUCCESS_RESPONSE":
            return {
                ...state,
                data: action.data,
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
            return {
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
            };
        default:
            return state;
    }
};

export default response;
