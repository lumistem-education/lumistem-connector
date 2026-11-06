package events

import ( 
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
)

type Client struct {
    mu sync.Mutex
    w   http.ResponseWriter
    rc  *http.ResponseController
    closed bool
    done chan struct{}
}

func NewClient(w http.ResponseWriter) *Client {
    return &Client{
        w: w,
        rc: http.NewResponseController(w),
        done: make(chan struct{}),
    }
}

func (c *Client) Done() <-chan struct{} {
    return c.done
}

func (c *Client) Send(ev Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
 
	if c.closed {
		return fmt.Errorf("client closed")
	}
 
	data, err := json.Marshal(ev.Data)
	if err != nil {
		return err
	}
 
	if _, err := fmt.Fprintf(c.w, "event: %s\n", ev.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.w, "data: %s\n\n", data); err != nil {
		return err
	}
 
	return c.rc.Flush()
}

func (c *Client) Ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()
 
	if c.closed {
		return fmt.Errorf("client closed")
	}
	if _, err := fmt.Fprint(c.w, ": ping\n\n"); err != nil {
		return err
	}

	return c.rc.Flush()
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
 
	if c.closed {
		return nil
	}

	c.closed = true
	close(c.done)

	return nil
}
