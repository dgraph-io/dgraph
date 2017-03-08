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
            // TODO - There has to be a cleaner way to assign these default values.
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
            };
        default:
            return state;
    }
};

export default response;
