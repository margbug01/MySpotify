// Package qtweb owns the Linux WebEngine process shared by sign-in and audio.
package qtweb

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"vinyl/internal/l10n"
)

var makeHostCommand = hostCommand
var idleGrace = 20 * time.Second

var shared struct {
	sync.Mutex
	client *Client
	users  int
	idle   *time.Timer
}

// Enabled reports whether Linux sign-in and audio use the shared WebEngine host.
func Enabled() bool { return enabled() }

func sharedLocked() (*Client, error) {
	if shared.client != nil && shared.client.alive() {
		return shared.client, nil
	}
	if shared.idle != nil {
		shared.idle.Stop()
		shared.idle = nil
	}
	shared.client, shared.users = nil, 0
	command, err := makeHostCommand()
	if err != nil {
		return nil, err
	}
	client, err := start(command)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := client.Call(ctx, "ping", nil); err != nil {
		client.Close()
		return nil, fmt.Errorf(l10n.T("Qt WebEngine 启动失败，请检查运行时及显示服务: %w"), err)
	}
	shared.client = client
	return client, nil
}

// Acquire keeps the shared host alive while an audio page or capture owns it.
// The last release reclaims it after a grace period; a new owner cancels that close.
func Acquire() (*Client, func(), error) {
	shared.Lock()
	defer shared.Unlock()
	client, err := sharedLocked()
	if err != nil {
		return nil, nil, err
	}
	if shared.idle != nil {
		shared.idle.Stop()
		shared.idle = nil
	}
	shared.users++
	var once sync.Once
	release := func() {
		once.Do(func() {
			shared.Lock()
			defer shared.Unlock()
			if shared.client != client || shared.users == 0 {
				return
			}
			shared.users--
			if shared.users != 0 {
				return
			}
			shared.idle = time.AfterFunc(idleGrace, func() { reclaimIdle(client) })
		})
	}
	return client, release, nil
}

func reclaimIdle(client *Client) {
	shared.Lock()
	defer shared.Unlock()
	if shared.client == client && shared.users == 0 {
		if shared.idle != nil {
			shared.idle.Stop()
		}
		// Keep the pool locked until the old profile has closed.
		client.Close()
		shared.client, shared.idle = nil, nil
	}
}

func Shutdown() {
	shared.Lock()
	client := shared.client
	shared.client, shared.users = nil, 0
	if shared.idle != nil {
		shared.idle.Stop()
		shared.idle = nil
	}
	shared.Unlock()
	if client != nil {
		client.Close()
	}
}

type reply struct {
	ID    int             `json:"id"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

// Client uses private stdio, never a public endpoint, for identity and cookie data.
// Neither protocol values nor remote page console output are logged.
type Client struct {
	command *exec.Cmd
	input   io.WriteCloser
	done    chan struct{}
	writeMu sync.Mutex
	mu      sync.Mutex
	next    int
	err     error
	pending map[int]chan reply
	watch   map[int]func()
	close   sync.Once
}

func start(command *exec.Cmd) (*Client, error) {
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	if err := command.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	c := &Client{command: command, input: input, done: make(chan struct{}), pending: map[int]chan reply{}, watch: map[int]func(){}}
	go func() {
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), 2<<20)
		for scanner.Scan() {
			var r reply
			if json.Unmarshal(scanner.Bytes(), &r) != nil || r.ID <= 0 {
				continue
			}
			c.mu.Lock()
			waiting := c.pending[r.ID]
			delete(c.pending, r.ID)
			c.mu.Unlock()
			if waiting != nil {
				waiting <- r
			}
		}
		// EOF also stops a host whose protocol reader has failed.
		input.Close()
		// Wait only after the output has drained; otherwise Wait can close its pipe early.
		command.Wait()
		c.mu.Lock()
		c.err = errors.New(l10n.T("Qt WebEngine 进程已退出"))
		watchers := c.watch
		c.watch = map[int]func(){}
		c.pending = map[int]chan reply{}
		close(c.done)
		c.mu.Unlock()
		for _, fn := range watchers {
			fn()
		}
	}()
	return c, nil
}

func (c *Client) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func (c *Client) Call(ctx context.Context, method string, args any) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return nil, err
	}
	c.next++
	id := c.next
	waiting := make(chan reply, 1)
	c.pending[id] = waiting
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()
	request, err := json.Marshal(struct {
		ID     int    `json:"id"`
		Method string `json:"method"`
		Args   any    `json:"args"`
	}{id, method, args})
	if err != nil {
		return nil, err
	}
	c.writeMu.Lock()
	if err = ctx.Err(); err == nil {
		if pipe, ok := c.input.(interface{ SetWriteDeadline(time.Time) error }); ok {
			deadline, bounded := ctx.Deadline()
			if !bounded {
				deadline = time.Now().Add(10 * time.Second)
			}
			pipe.SetWriteDeadline(deadline)
		}
		_, err = c.input.Write(append(request, '\n'))
	}
	c.writeMu.Unlock()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New(l10n.T("Qt WebEngine 通道已关闭"))
	}
	select {
	case response := <-waiting:
		if response.Error != "" {
			return nil, errors.New(l10n.T(response.Error))
		}
		return response.Value, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, errors.New(l10n.T("Qt WebEngine 进程已退出"))
	}
}

// OnExit returns an unsubscribe function. A late subscriber is notified too.
func (c *Client) OnExit(fn func()) func() {
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		fn()
		return func() {}
	}
	c.next++
	id := c.next
	c.watch[id] = fn
	c.mu.Unlock()
	return func() { c.mu.Lock(); delete(c.watch, id); c.mu.Unlock() }
}

func (c *Client) Close() {
	c.close.Do(func() {
		// EOF is the normal shutdown signal; the host closes its pages before quitting.
		// Closing an os.Pipe concurrently releases a writer blocked behind a hung host.
		c.input.Close()
		select {
		case <-c.done:
		case <-time.After(3 * time.Second):
			c.command.Process.Kill()
			<-c.done
		}
	})
}
