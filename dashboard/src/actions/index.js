import {
    // timeout,
    checkStatus,
    parseJSON,
    isNotEmpty,
    showTreeView, // isShortestPath,
} from "../containers/Helpers";

const addQuery = text => ({
    type: "ADD_QUERY",
    text,
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

export const runQuery = query => {
    return dispatch => {
        // TODO - Add timeout back
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
                console.log(result);
                // This is the case in which user sends a mutation. We display the response from server.
                if (result.code !== undefined && result.message !== undefined) {
                    dispatch(addQuery(query));
                    dispatch(saveSuccessResponse(JSON.stringify(result)));
                } else if (isNotEmpty(result)) {
                    dispatch(addQuery(query));
                    // let mantainSortOrder = showTreeView(query);
                    dispatch(saveSuccessResponse(null, result));
                } else {
                    dispatch(
                        saveErrorResponse(
                            "Your query did not return any results.",
                        ),
                    );
                }
            })
            .catch(function(error) {
                console.log(error.stack);
                var err = error.response &&
                    (error.response.text() || error.message);
                return err;
            })
            .then(function(errorMsg) {
                if (errorMsg !== undefined) {
                    dispatch(saveErrorResponse(errorMsg));
                }
            });
    };
};

// export const
