export const UPDATE_QUERY = "query/UPDATE_QUERY";

export function updateQuery(query) {
  return {
    type: UPDATE_QUERY,
    query
  };
}
