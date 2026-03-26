package stats

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Collector defines the interface for collecting game statistics events.
type Collector interface {
	RecordShot(ctx context.Context, event ShotEvent) error
	RecordKill(ctx context.Context, event KillEvent) error
	RecordSession(ctx context.Context, event SessionEvent) error
	RecordRoom(ctx context.Context, event RoomEvent) error
	Flush(ctx context.Context) error
	Close() error
}

// CollectorConfig holds configuration for the buffered collector.
type CollectorConfig struct {
	RoomID        string
	Endpoint      string        // HTTP endpoint to POST events
	BufferSize    int           // Max events in buffer before dropping oldest
	FlushInterval time.Duration // Time interval to flush buffer
	FlushBatch    int           // Flush when buffer reaches this size
}

// bufferedCollector implements Collector with event buffering and HTTP posting.
type bufferedCollector struct {
	config      CollectorConfig
	buffer      []Event
	mu          sync.Mutex
	flushTicker *time.Ticker
	client      *http.Client
	done        chan struct{}
	wg          sync.WaitGroup
}

// NewCollector creates a new buffered collector with automatic flushing.
func NewCollector(config CollectorConfig) Collector {
	// Set defaults
	if config.BufferSize == 0 {
		config.BufferSize = 1000
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = 1 * time.Second
	}
	if config.FlushBatch == 0 {
		config.FlushBatch = 100
	}

	c := &bufferedCollector{
		config:      config,
		buffer:      make([]Event, 0, config.BufferSize),
		flushTicker: time.NewTicker(config.FlushInterval),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		done: make(chan struct{}),
	}

	// Start background flush goroutine
	c.wg.Add(1)
	go c.flushLoop()

	return c
}

// RecordShot records a weapon fire event.
func (c *bufferedCollector) RecordShot(ctx context.Context, event ShotEvent) error {
	return c.addEvent(Event{Type: "shot", Data: event})
}

// RecordKill records a kill event.
func (c *bufferedCollector) RecordKill(ctx context.Context, event KillEvent) error {
	return c.addEvent(Event{Type: "kill", Data: event})
}

// RecordSession records a session start or end event.
func (c *bufferedCollector) RecordSession(ctx context.Context, event SessionEvent) error {
	return c.addEvent(Event{Type: "session", Data: event})
}

// RecordRoom records a room lifecycle event.
func (c *bufferedCollector) RecordRoom(ctx context.Context, event RoomEvent) error {
	return c.addEvent(Event{Type: "room", Data: event})
}

// addEvent adds an event to the buffer, dropping oldest if full.
func (c *bufferedCollector) addEvent(event Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if buffer is full
	if len(c.buffer) >= c.config.BufferSize {
		// Drop oldest event
		log.Printf("stats: buffer full (%d events), dropping oldest event", c.config.BufferSize)
		c.buffer = c.buffer[1:]
	}

	c.buffer = append(c.buffer, event)

	// Trigger immediate flush if batch size reached
	if len(c.buffer) >= c.config.FlushBatch {
		go c.flush()
	}

	return nil
}

// flushLoop runs in background and flushes buffer periodically.
func (c *bufferedCollector) flushLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.done:
			// Final flush before shutdown
			c.flush()
			return
		case <-c.flushTicker.C:
			c.flush()
		}
	}
}

// flush sends all buffered events to the room-manager endpoint.
func (c *bufferedCollector) flush() {
	c.mu.Lock()
	if len(c.buffer) == 0 {
		c.mu.Unlock()
		return
	}

	// Copy buffer and clear
	events := make([]Event, len(c.buffer))
	copy(events, c.buffer)
	c.buffer = c.buffer[:0]
	c.mu.Unlock()

	// Send events via HTTP POST
	if err := c.sendEvents(events); err != nil {
		log.Printf("stats: failed to send %d events: %v", len(events), err)
		// Events are lost, but game continues
	}
}

// sendEvents sends a batch of events to the room-manager endpoint.
func (c *bufferedCollector) sendEvents(events []Event) error {
	if c.config.Endpoint == "" {
		return fmt.Errorf("no endpoint configured")
	}

	// Marshal events to JSON
	payload := map[string]interface{}{
		"events": events,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	// POST to room-manager
	resp, err := c.client.Post(c.config.Endpoint, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("HTTP POST failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Flush immediately sends all buffered events.
func (c *bufferedCollector) Flush(ctx context.Context) error {
	c.flush()
	return nil
}

// Close gracefully shuts down the collector, flushing remaining events.
func (c *bufferedCollector) Close() error {
	// Stop flush ticker
	c.flushTicker.Stop()

	// Signal flush loop to stop
	close(c.done)

	// Wait for flush loop to finish (includes final flush)
	c.wg.Wait()

	return nil
}

// noopCollector is a no-op implementation for when statistics are disabled.
type noopCollector struct{}

// NewNoopCollector creates a collector that does nothing.
func NewNoopCollector() Collector {
	return &noopCollector{}
}

func (n *noopCollector) RecordShot(ctx context.Context, event ShotEvent) error {
	return nil
}

func (n *noopCollector) RecordKill(ctx context.Context, event KillEvent) error {
	return nil
}

func (n *noopCollector) RecordSession(ctx context.Context, event SessionEvent) error {
	return nil
}

func (n *noopCollector) RecordRoom(ctx context.Context, event RoomEvent) error {
	return nil
}

func (n *noopCollector) Flush(ctx context.Context) error {
	return nil
}

func (n *noopCollector) Close() error {
	return nil
}
