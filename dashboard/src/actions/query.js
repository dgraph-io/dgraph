export const UPDATE_QUERY = "query/UPDATE_QUERY";
export const UPDATE_ACTION = "query/UPDATE_ACTION";
export const UPDATE_QUERY_AND_ACTION = "query/UPDATE_QUERY_AND_ACTION";

export function updateQuery(query) {
	return {
		type: UPDATE_QUERY,
		query
	};
}

export function updateAction(action) {
	return {
		type: UPDATE_ACTION,
		action
	};
}

export function updateQueryAndAction(query, action) {
	return {
		type: UPDATE_QUERY_AND_ACTION,
		query,
		action
	};
}
