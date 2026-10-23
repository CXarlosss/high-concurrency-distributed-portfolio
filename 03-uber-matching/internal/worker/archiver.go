package worker

import (
	"context"
	"errors"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cv-projects/uber-matching/internal/storage"
	"github.com/redis/go-redis/v9"
)

const (
	DefaultStreamKey   = "trips:events"
	DefaultGroupName   = "trips-archiver-group"
	DefaultConsumerID  = "archiver-worker-1"
	DefaultBatchSize   = 100
	DefaultPollTimeout = 500 * time.Millisecond
)

type ArchiverWorker struct {
	redisClient  *redis.Client
	db           storage.DBStore
	streamKey    string
	groupName    string
	consumerID   string
	batchSize    int64
	pollTimeout  time.Duration
	stopChan     chan struct{}
	wg           sync.WaitGroup
	running      int32

	// Telemetry
	processedCount uint64
	failedBatches  uint64
	retriedCount   uint64
}

func NewArchiverWorker(client *redis.Client, db storage.DBStore) *ArchiverWorker {
	return &ArchiverWorker{
		redisClient: client,
		db:          db,
		streamKey:   DefaultStreamKey,
		groupName:   DefaultGroupName,
		consumerID:  DefaultConsumerID,
		batchSize:   DefaultBatchSize,
		pollTimeout: DefaultPollTimeout,
		stopChan:    make(chan struct{}),
	}
}

func (w *ArchiverWorker) ProcessedCount() uint64 {
	return atomic.LoadUint64(&w.processedCount)
}

func (w *ArchiverWorker) FailedBatches() uint64 {
	return atomic.LoadUint64(&w.failedBatches)
}

// InitConsumerGroup initializes the stream and consumer group idempotently.
func (w *ArchiverWorker) InitConsumerGroup(ctx context.Context) error {
	err := w.redisClient.XGroupCreateMkStream(ctx, w.streamKey, w.groupName, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

// Start launches the consumer loop in a background goroutine.
func (w *ArchiverWorker) Start() {
	if !atomic.CompareAndSwapInt32(&w.running, 0, 1) {
		return
	}

	w.wg.Add(1)
	go w.consumeLoop()
}

// Stop executes a graceful shutdown: stops consuming, drains pending buffer, and exits.
func (w *ArchiverWorker) Stop(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&w.running, 1, 0) {
		return
	}

	log.Printf("[Archiver] Graceful shutdown initiated. Draining pending events...")
	close(w.stopChan)

	c := make(chan struct{})
	go func() {
		w.wg.Wait()
		w.db.Close()
		close(c)
	}()

	select {
	case <-c:
		log.Printf("[Archiver] Shutdown complete. All batches flushed and DB pool closed.")
	case <-ctx.Done():
		log.Printf("[Archiver] Shutdown timed out. Forcing termination.")
	}
}

func (w *ArchiverWorker) consumeLoop() {
	defer w.wg.Done()

	for {
		select {
		case <-w.stopChan:
			return
		default:
			processedAny := w.processBatch()
			if !processedAny {
				time.Sleep(20 * time.Millisecond)
			}
		}
	}
}

func (w *ArchiverWorker) processBatch() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. First, check for pending messages (PEL) that failed during a previous DB crash
	streams, err := w.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    w.groupName,
		Consumer: w.consumerID,
		Streams:  []string{w.streamKey, "0"}, // "0" reads pending unacked messages
		Count:    w.batchSize,
	}).Result()

	isPending := true
	if err != nil || len(streams) == 0 || len(streams[0].Messages) == 0 {
		isPending = false
		// 2. If no pending messages, read new incoming messages (">")
		streams, err = w.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    w.groupName,
			Consumer: w.consumerID,
			Streams:  []string{w.streamKey, ">"},
			Count:    w.batchSize,
			Block:    w.pollTimeout,
		}).Result()
	}

	if err != nil {
		if !errors.Is(err, redis.Nil) {
			atomic.AddUint64(&w.failedBatches, 1)
		}
		return false
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return false
	}

	messages := streams[0].Messages
	events := make([]storage.TripEvent, 0, len(messages))
	msgIDs := make([]string, 0, len(messages))

	for _, msg := range messages {
		msgIDs = append(msgIDs, msg.ID)

		tripID, _ := msg.Values["trip_id"].(string)
		driverID, _ := msg.Values["driver_id"].(string)
		state, _ := msg.Values["state"].(string)
		tsRaw, _ := msg.Values["timestamp"].(string)

		tsNano, _ := strconv.ParseInt(tsRaw, 10, 64)
		ts := time.Unix(0, tsNano)
		if ts.IsZero() {
			ts = time.Now()
		}

		events = append(events, storage.TripEvent{
			EventID:   msg.ID,
			TripID:    tripID,
			DriverID:  driverID,
			State:     state,
			Timestamp: ts,
		})
	}

	// 3. Write batch to PostgreSQL
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer dbCancel()

	if err := w.db.SaveEventsBatch(dbCtx, events); err != nil {
		// CRITICAL RESILIENCE RULE:
		// If PostgreSQL fails, DO NOT call XACK!
		// Leave messages in Redis PEL so they can be retried upon DB recovery.
		atomic.AddUint64(&w.failedBatches, 1)
		time.Sleep(100 * time.Millisecond) // Backoff to avoid hot spinning while DB is down
		return true
	}

	// 4. Acknowledge messages in Redis ONLY after PostgreSQL COMMIT succeeds!
	if err := w.redisClient.XAck(ctx, w.streamKey, w.groupName, msgIDs...).Err(); err != nil {
		log.Printf("[Archiver] Warning: XACK failed: %v", err)
	}

	if isPending {
		atomic.AddUint64(&w.retriedCount, uint64(len(messages)))
	}
	atomic.AddUint64(&w.processedCount, uint64(len(messages)))
	return true
}
