package json2

type ErrCode int

// Offical JSON-RPC 2.0 Error codes
// https://www.jsonrpc.org/specification#error_object
const (
	ERR_PARSE          ErrCode = -32700
	ERR_INVALID_REQ    ErrCode = -32600
	ERR_INVALID_METHOD ErrCode = -32601
	ERR_INVALID_PARAMS ErrCode = -32602
	ERR_INTERNAL_ERROR ErrCode = -32603
	// -32000 to -32099 are reserved for implementation specific errors
)

type Error struct {
	Message   string
	ErrorCode ErrCode
}

func (e *Error) Error() string {
	return e.Message
}
