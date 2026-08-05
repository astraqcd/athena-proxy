package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const requestTimeout = 10 * time.Second

var ErrNoDaemon = errors.New("no athena-proxy daemon is running")

type Client struct {
	port int
	http *http.Client
}

func NewClient(port int) *Client {
	return &Client{
		port: port,
		http: &http.Client{Timeout: requestTimeout},
	}
}

func (c *Client) Port() int {
	return c.port
}

func (c *Client) Status() (Status, error) {
	var out Status
	err := c.do(http.MethodGet, PathStatus, nil, &out)
	if err != nil {
		return Status{}, err
	}
	if out.Service != Service {
		return Status{}, ErrNoDaemon
	}
	return out, nil
}

func (c *Client) Add(req AddRequest) (AddResponse, error) {
	var out AddResponse
	return out, c.do(http.MethodPost, PathTargets, req, &out)
}

func (c *Client) List() (ListResponse, error) {
	var out ListResponse
	return out, c.do(http.MethodGet, PathTargets, nil, &out)
}

func (c *Client) Remove(selector string) (RemoveResponse, error) {
	var out RemoveResponse
	path := PathTargets + "/" + url.PathEscape(selector)
	return out, c.do(http.MethodDelete, path, nil, &out)
}

func (c *Client) do(method, path string, body, out any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(c.port))
	req, err := http.NewRequestWithContext(ctx, method, "http://"+addr+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			return ErrNoDaemon
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr Error
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil && apiErr.Message != "" {
			return errors.New(apiErr.Message)
		}
		return fmt.Errorf("daemon returned %s", resp.Status)
	}

	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
