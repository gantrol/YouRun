package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Native content uses the same authenticated, bounded host API as the two web
// clients. No HTML assets, browser runtime or JavaScript bridge are loaded.
type client struct {
	address, token string
	http           *http.Client
	ctx            context.Context
}

func newClient(ctx context.Context, address string) *client {
	u, _ := url.Parse(address)
	return &client{ctx: ctx, address: u.Scheme + "://" + u.Host, token: u.Fragment,
		http: &http.Client{Transport: &http.Transport{Proxy: nil},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *client) request(method, path string, input, output any) error {
	return c.requestContext(c.ctx, method, path, input, output)
}

func (c *client) requestContext(ctx context.Context, method, path string, input, output any) error {
	var body []byte
	var err error
	if input != nil {
		body, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	// Folder dialogs are user-controlled. Other requests have a finite deadline.
	if path != "choose-directory" && path != "updates/local" && path != "updates/source" && path != "project-avatar" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}
	r, err := http.NewRequestWithContext(ctx, method, c.address+"/api/"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(r)
	if err != nil {
		return errors.New("connectionLost")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return errors.New("connectionLost")
	}
	if response.StatusCode != http.StatusOK {
		var result struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &result)
		if result.Error == "" {
			result.Error = "requestFailed"
		}
		return errors.New(result.Error)
	}
	if output != nil {
		return json.Unmarshal(data, output)
	}
	return nil
}

// subscribe owns one authenticated SSE connection at a time. Only connection
// deadlines and reconnect backoff use timers; neither timer reads application data.
// receive is synchronous so the consumer can apply backpressure to the stream.
func (c *client) subscribe(ctx context.Context, path func() string, receive func([]byte) error, failed func(error)) {
	delay := time.Second
	for ctx.Err() == nil {
		err := c.readEvents(ctx, path(), func(data []byte) error {
			if err := receive(data); err != nil {
				return err
			}
			delay = time.Second
			return nil
		})
		if ctx.Err() != nil {
			return
		}
		failed(err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(delay*2, 15*time.Second)
	}
}

func (c *client) readEvents(parent context.Context, path string, receive func([]byte) error) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	deadline := time.AfterFunc(15*time.Second, cancel)
	defer deadline.Stop()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, c.address+"/api/"+path, nil)
	if err != nil {
		return err
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Accept", "text/event-stream")
	response, err := c.http.Do(r)
	if err != nil {
		return errors.New("connectionLost")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || strings.TrimSpace(strings.SplitN(response.Header.Get("Content-Type"), ";", 2)[0]) != "text/event-stream" {
		return errors.New("connectionLost")
	}
	const maxFrame = 16 << 20
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), maxFrame)
	var data strings.Builder
	for scanner.Scan() {
		deadline.Reset(45 * time.Second)
		line := scanner.Text()
		if line == "" {
			if data.Len() > 0 {
				if err := receive([]byte(data.String())); err != nil {
					return err
				}
				data.Reset()
			}
			continue
		}
		if line == "data" || strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(strings.TrimPrefix(line, "data"), ":")
			value = strings.TrimPrefix(value, " ")
			if data.Len()+len(value)+1 > maxFrame {
				return errors.New("requestFailed")
			}
			data.WriteString(value)
			data.WriteByte('\n')
		}
	}
	return errors.New("connectionLost")
}
