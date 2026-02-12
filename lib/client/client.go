package client

import "github.com/wolves-fc/tasker/lib/rpc"

// Client connects to a Tasker server and provides RPC methods.
type Client struct {
	conn *rpc.Client
}

// New creates a new client connected to the given server address with mutual TLS.
func New(certDir, user, addr string) (*Client, error) {
	conn, err := rpc.Dial(certDir, user, addr)
	if err != nil {
		return nil, err
	}

	return &Client{conn: conn}, nil
}

// Close disconnects from the server.
func (c *Client) Close() error {
	return c.conn.Close()
}
