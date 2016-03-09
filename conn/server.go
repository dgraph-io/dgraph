package conn

import (
	"io"
	"log"
	"net/rpc"
)

type ServerCodec struct {
	Rwc        io.ReadWriteCloser
	payloadLen int32
}

func (c *ServerCodec) ReadRequestHeader(r *rpc.Request) error {
	return parseHeader(c.Rwc, &r.Seq, &r.ServiceMethod, &c.payloadLen)
}

func (c *ServerCodec) ReadRequestBody(data interface{}) error {
	b := make([]byte, c.payloadLen)
	_, err := io.ReadFull(c.Rwc, b)
	if err != nil {
		return err
	}

	if data == nil {
		// If data is nil, discard this request.
		return nil
	}
	query := data.(*Query)
	query.Data = b
	return nil
}

func (c *ServerCodec) WriteResponse(resp *rpc.Response,
	data interface{}) error {

	if len(resp.Error) > 0 {
		log.Fatal("Response has error: " + resp.Error)
	}
	if data == nil {
		log.Fatal("Worker write response data is nil")
	}
	reply, ok := data.(*Reply)
	if !ok {
		log.Fatal("Unable to convert to reply")
	}

	if err := writeHeader(c.Rwc, resp.Seq,
		resp.ServiceMethod, reply.Data); err != nil {
		return err
	}

	_, err := c.Rwc.Write(reply.Data)
	return err
}

func (c *ServerCodec) Close() error {
	return c.Rwc.Close()
}
