package graphql

func OneShot(resp *Response) func() *Response {
	var oneshot bool

	return func() *Response {
		if oneshot {
			return nil
		}
		oneshot = true

		return resp
	}
}
