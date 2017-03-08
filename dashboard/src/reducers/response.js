const response = (
    state = {
        text: "",
        data: {},
        success: false,
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
                text: JSON.stringify(action.data) || "",
                data: action.data || {},
                success: true,
            };
        default:
            return state;
    }
};

export default response;
