package bidx

// TODO(jiawei)
// It is not clear how frontfill should be implemented.
// Should it be hooked to commit logs? Or hooked to worker receiving mutations?
// I would think it is better to connect to commit logs which runs every 1s and
// we can batch indexing over one second.
