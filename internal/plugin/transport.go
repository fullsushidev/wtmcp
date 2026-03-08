package plugin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
)

// Transport manages bidirectional JSON-lines communication with a
// plugin process over stdin/stdout.
type Transport struct {
	stdin   io.Writer
	stdout  io.Reader
	stderr  io.Reader
	mu      sync.Mutex // serialize writes to stdin
	pending sync.Map   // id -> chan Message
	maxSize int        // max message size in bytes
	nextID  atomic.Int64
	done    chan struct{} // closed when ReadLoop exits
}

// NewTransport creates a Transport for communicating with a plugin process.
func NewTransport(stdin io.Writer, stdout, stderr io.Reader, maxSize int) *Transport {
	return &Transport{
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		maxSize: maxSize,
		done:    make(chan struct{}),
	}
}

// GenerateID returns a unique message ID for service requests.
func (t *Transport) GenerateID(prefix string) string {
	n := t.nextID.Add(1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// Send writes a JSON message to the plugin's stdin.
// Thread-safe: serializes writes via mutex to guarantee atomic lines.
func (t *Transport) Send(msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	data = append(data, '\n')

	t.mu.Lock()
	defer t.mu.Unlock()

	_, err = t.stdin.Write(data)
	return err
}

// SendAndWait sends a message and waits for a response with the same ID.
// The response is routed by ReadLoop.
func (t *Transport) SendAndWait(id string, msg Message) (Message, error) {
	ch := make(chan Message, 1)
	t.pending.Store(id, ch)
	defer t.pending.Delete(id)

	msg.ID = id
	if err := t.Send(msg); err != nil {
		return Message{}, fmt.Errorf("send: %w", err)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return Message{}, fmt.Errorf("plugin exited while waiting for response to %s", id)
		}
		return resp, nil
	case <-t.done:
		return Message{}, fmt.Errorf("transport closed while waiting for response to %s", id)
	}
}

// ReadLoop reads messages from the plugin's stdout and routes them.
//
// For concurrency <= 1, service requests (http_request, cache_*) are
// handled synchronously — no goroutines, no race conditions. This
// guarantees that sequential plugins can use simple blocking read/write
// loops.
//
// For concurrency > 1, service requests are handled in goroutines.
//
// The handler functions are provided by the caller (proxy and cache).
func (t *Transport) ReadLoop(pluginName string, concurrency int, serviceHandler ServiceHandler) {
	defer close(t.done)

	scanner := bufio.NewScanner(t.stdout)
	scanner.Buffer(make([]byte, 0), t.maxSize)

	for scanner.Scan() {
		var msg Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			log.Printf("[%s] malformed message: %v", pluginName, err)
			continue
		}

		switch msg.Type {
		case TypeHTTPRequest:
			if concurrency <= 1 {
				resp := serviceHandler.HandleHTTP(pluginName, msg)
				if err := t.Send(resp); err != nil {
					log.Printf("[%s] failed to send http_response: %v", pluginName, err)
				}
			} else {
				go func(m Message) {
					resp := serviceHandler.HandleHTTP(pluginName, m)
					if err := t.Send(resp); err != nil {
						log.Printf("[%s] failed to send http_response: %v", pluginName, err)
					}
				}(msg)
			}

		case TypeCacheGet, TypeCacheSet, TypeCacheDel, TypeCacheList, TypeCacheFlush:
			if concurrency <= 1 {
				resp := serviceHandler.HandleCache(pluginName, msg)
				if err := t.Send(resp); err != nil {
					log.Printf("[%s] failed to send cache response: %v", pluginName, err)
				}
			} else {
				go func(m Message) {
					resp := serviceHandler.HandleCache(pluginName, m)
					if err := t.Send(resp); err != nil {
						log.Printf("[%s] failed to send cache response: %v", pluginName, err)
					}
				}(msg)
			}

		case TypeToolResult, TypeInitOK, TypeInitError, TypeShutdownOK, TypeAuthResponse:
			if ch, ok := t.pending.LoadAndDelete(msg.ID); ok {
				ch.(chan Message) <- msg
			}

		default:
			log.Printf("[%s] unknown message type: %q (id: %s)", pluginName, msg.Type, msg.ID)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[%s] read error: %v", pluginName, err)
	}

	// Drain pending channels so blocked callers get immediate errors.
	t.pending.Range(func(key, value any) bool {
		close(value.(chan Message))
		t.pending.Delete(key)
		return true
	})
}

// ForwardStderr reads the plugin's stderr and logs it with a prefix.
func (t *Transport) ForwardStderr(pluginName string) {
	scanner := bufio.NewScanner(t.stderr)
	for scanner.Scan() {
		log.Printf("[%s] %s", pluginName, scanner.Text())
	}
}

// ServiceHandler handles service requests from plugins.
// Implemented by the proxy and cache subsystems.
type ServiceHandler interface {
	HandleHTTP(pluginName string, req Message) Message
	HandleCache(pluginName string, req Message) Message
}
