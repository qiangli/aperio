package export

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Batcher accumulates OTLP spans and flushes them in batches.
type Batcher struct {
	opts      SendOptions
	resource  Resource
	scope     InstrumentationScope
	traceID   string
	threshold int
	interval  time.Duration

	mu     sync.Mutex
	spans  []OTLPSpan
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// BatcherConfig configures the batcher.
type BatcherConfig struct {
	// Threshold is the number of spans that triggers an immediate flush (default: 100).
	Threshold int
	// Interval is the periodic flush interval (default: 5s).
	Interval time.Duration
}

// NewBatcher creates a new span batcher.
func NewBatcher(opts SendOptions, resource Resource, scope InstrumentationScope, traceID string, cfg BatcherConfig) *Batcher {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 100
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}

	return &Batcher{
		opts:      opts,
		resource:  resource,
		scope:     scope,
		traceID:   traceID,
		threshold: cfg.Threshold,
		interval:  cfg.Interval,
		stopCh:    make(chan struct{}),
	}
}

// Add adds a span to the batch. If the batch reaches the threshold, it is flushed.
func (b *Batcher) Add(span OTLPSpan) {
	b.mu.Lock()
	b.spans = append(b.spans, span)
	shouldFlush := len(b.spans) >= b.threshold
	b.mu.Unlock()

	if shouldFlush {
		if err := b.Flush(); err != nil {
			log.Error().Err(err).Msg("batcher: flush on threshold failed")
		}
	}
}

// Flush sends all accumulated spans to the OTLP endpoint.
func (b *Batcher) Flush() error {
	b.mu.Lock()
	if len(b.spans) == 0 {
		b.mu.Unlock()
		return nil
	}
	spans := b.spans
	b.spans = nil
	b.mu.Unlock()

	req := &OTLPExportRequest{
		ResourceSpans: []ResourceSpans{
			{
				Resource: b.resource,
				ScopeSpans: []ScopeSpans{
					{
						Scope: b.scope,
						Spans: spans,
					},
				},
			},
		},
	}

	return Send(req, b.opts)
}

// Start begins periodic flushing in a background goroutine.
func (b *Batcher) Start() {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		ticker := time.NewTicker(b.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := b.Flush(); err != nil {
					log.Error().Err(err).Msg("batcher: periodic flush failed")
				}
			case <-b.stopCh:
				return
			}
		}
	}()
}

// Stop flushes remaining spans and stops the periodic flusher.
func (b *Batcher) Stop() error {
	close(b.stopCh)
	b.wg.Wait()
	return b.Flush()
}

// Pending returns the number of spans waiting to be flushed.
func (b *Batcher) Pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.spans)
}
