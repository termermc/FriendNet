package clog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"friendnet.org/common"
	"github.com/google/uuid"
)

// The current serialization version of message metadata.
const curMetaSerialVer = 1

// The length to use for the message buffer channel.
const msgBufLen = 1024

type Attr struct {
	Kind  string `json:"kind"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MessageRecord is a record of a log message to be stored in or from the database.
type MessageRecord struct {
	// The message's UUID.
	// If passing to Handler.write, can be empty to be filled by the method.
	Uuid string

	// The message's creation timestamp.
	// Uses millisecond resolution; smaller measurements will not be stored.
	// If passing to Handler.write, can be empty to be filled by the method.
	CreatedTs time.Time

	// The client run ID associated with the message.
	// Each successive run ID should be greater than the last, so it should
	// be a timestamp with sufficient resolution to avoid conflicts.
	RunId int64

	// The associated log level.
	Level slog.Level

	// The message.
	Message string

	// The message attributes.
	Attrs []Attr
}

// SubscriberFunc is a function that handles new log messages.
// It is run in its own goroutine.
type SubscriberFunc func(msg MessageRecord)

// SubscriptionId is an identifier for a log message subscription.
// It is used to unsubscribe.
type SubscriptionId struct {
	string
}

type subscription struct {
	id SubscriptionId
	fn SubscriberFunc
}

type subMgr struct {
	mu sync.RWMutex

	subscriptions []subscription
}

// Handler provides a slog.Handler interface for the client logger.
// You can subscribe to new messages by calling Subscribe and then Unsubscribe later to remove the subscription.
//
// It uses the client's SQLite instance.
// It relies on the migrations in the migrations module, so it is not standalone.
type Handler struct {
	printHandler slog.Handler

	db    *sql.DB
	runId int64

	subMgr *subMgr

	// A buffered channel of messages to process in a separate goroutine.
	msgBuf chan MessageRecord
	// A channel closed when all pending messages have been processed.
	drained chan struct{}

	attrKeyPrefix string
	// The attributes to add to every message.
	attrs []slog.Attr
}

// NewHandler creates a new Handler.
// The printHandler arg is the handler to use for printing to the console.
// The runId arg is the client run ID, which should be a value that increases each time the client is run (such as a UNIX millisecond timestamp).
func NewHandler(db *sql.DB, runId int64, printHandler slog.Handler) Handler {
	h := Handler{
		printHandler: printHandler,

		db:    db,
		runId: runId,

		subMgr: &subMgr{},

		msgBuf:  make(chan MessageRecord, msgBufLen),
		drained: make(chan struct{}),
	}

	go h.processor()

	return h
}

func (h Handler) processor() {
	for rec := range h.msgBuf {
		err := h.write(rec)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to write buffered log message: %v\n", err)
		}

		// Launch goroutines for subscriptions.
		h.subMgr.mu.RLock()
		for _, sub := range h.subMgr.subscriptions {
			go func() {
				defer func() {
					if recovery := recover(); recovery != nil {
						_, _ = fmt.Fprintf(os.Stderr, "panic in log message subscription: %v\n\n%s\n",
							recovery,
							string(debug.Stack()),
						)
					}
				}()

				sub.fn(rec)
			}()
		}
		h.subMgr.mu.RUnlock()
	}
	close(h.drained)
}

// Close closes the logger and ensures that any pending messages are written before returning.
// It never returns an error.
func (h Handler) Close() error {
	close(h.msgBuf)
	<-h.drained
	return nil
}

// Subscribe adds a new message subscription.
// The passed function will be called in its own goroutine for each new message.
func (h Handler) Subscribe(fn SubscriberFunc) SubscriptionId {
	id := SubscriptionId{common.RandomB64UrlStr(4)}

	h.subMgr.mu.Lock()
	h.subMgr.subscriptions = append(h.subMgr.subscriptions, subscription{
		id: id,
		fn: fn,
	})
	h.subMgr.mu.Unlock()

	return id
}

// Unsubscribe removes a message subscription.
func (h Handler) Unsubscribe(id SubscriptionId) {
	h.subMgr.mu.Lock()
	h.subMgr.subscriptions = slices.DeleteFunc(h.subMgr.subscriptions, func(sub subscription) bool {
		return sub.id != id
	})
	h.subMgr.mu.Unlock()
}

// GetLogsAfter returns log messages created after the specified timestamp (and with the current runId).
func (h Handler) GetLogsAfter(afterTs time.Time, minLevel slog.Level) ([]MessageRecord, error) {
	rows, err := h.db.Query(`select * from log where run_id = ? and level >= ? and created_ts > ?`,
		h.runId,
		minLevel,
		afterTs.UnixMilli(),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	recs := make([]MessageRecord, 0)
	for rows.Next() {
		var uid string
		var createdTs int64
		var runId int64
		var level slog.Level
		var message string
		var metaSerialVer int
		var metadata []byte

		err = rows.Scan(&uid, &createdTs, &runId, &level, &message, &metaSerialVer, &metadata)
		if err != nil {
			return nil, fmt.Errorf(`failed to scan log message row: %w`, err)
		}

		var attrs []Attr

		switch metaSerialVer {
		case 1:
			err = json.Unmarshal(metadata, &attrs)
			if err != nil {
				return nil, fmt.Errorf(`failed to unmarshal log message UUID %q attributes: %w`, uid, err)
			}
		default:
			// Unrecognized serial version.
			_, _ = fmt.Fprintf(os.Stderr, "encountered log message UUID %q with unknown metadata serialization version %d\n", uid, metaSerialVer)
			continue
		}

		rec := MessageRecord{
			Uuid:      uid,
			CreatedTs: time.UnixMilli(createdTs),
			RunId:     runId,
			Level:     level,
			Message:   message,
			Attrs:     attrs,
		}
		recs = append(recs, rec)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf(`error while iterating over log message rows: %w`, err)
	}

	return recs, nil
}

// slogAttrToAttrs converts a slog.Attr to a slice of Attrs.
// Flattens groups by prepending their key like "GROUP.", resulting in keys like "GROUP.otherAttr".
// The first call should use an empty prefix so that it can be replaced with h.attrKeyPrefix.
func (h Handler) slogAttrToAttrs(prefix string, attr slog.Attr) []Attr {
	if prefix == "" {
		prefix = h.attrKeyPrefix
	}

	if attr.Value.Kind() == slog.KindGroup {
		group := attr.Value.Group()
		res := make([]Attr, 0, len(group))
		for _, groupAttr := range group {
			res = append(res, h.slogAttrToAttrs(prefix+attr.Key+".", groupAttr)...)
		}
		return res
	}

	return []Attr{
		{
			Kind:  attr.Value.Kind().String(),
			Key:   prefix + attr.Key,
			Value: attr.Value.String(),
		},
	}
}

func (h Handler) write(rec MessageRecord) error {
	if rec.Uuid == "" {
		uuidRaw, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf(`failed to generate UUIDv7 in Handler.write: %w`, err)
		}

		rec.Uuid = uuidRaw.String()
	}
	if rec.CreatedTs.IsZero() {
		rec.CreatedTs = time.Now()
	}

	// Serialize attributes as a JSON array.
	attrsJson, err := json.Marshal(rec.Attrs)
	if err != nil {
		return fmt.Errorf(`failed to marshal log message metadata in Handler.write: %w`, err)
	}

	_, err = h.db.Exec(
		`insert into log (uuid, created_ts, run_id, level, message, metadata_serial_ver, metadata) values (?, ?, ?, ?, ?, ?, ?)`,
		rec.Uuid,
		rec.CreatedTs.UnixMilli(),
		rec.RunId,
		rec.Level,
		rec.Message,
		curMetaSerialVer,
		attrsJson,
	)
	if err != nil {
		return fmt.Errorf(`failed to insert log message in Handler.write: %w`, err)
	}

	return nil
}

func (h Handler) Enabled(_ context.Context, _ slog.Level) bool {
	// Handle all levels.
	return true
}

func (h Handler) Handle(ctx context.Context, record slog.Record) error {
	// Print first if supported level.
	if h.printHandler.Enabled(ctx, record.Level) {
		_ = h.printHandler.Handle(ctx, record)
	}

	// Construct message and attributes struct.
	attrs := make([]Attr, 0, len(h.attrs)+record.NumAttrs())
	for _, attr := range h.attrs {
		attrs = append(attrs, h.slogAttrToAttrs("", attr)...)
	}
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, h.slogAttrToAttrs("", attr)...)
		return true
	})
	rec := MessageRecord{
		RunId:     h.runId,
		CreatedTs: record.Time,
		Level:     record.Level,
		Message:   record.Message,
		Attrs:     attrs,
	}

	// This is considered EVIL practice, but I don't care.
	// I'd have to redo the way this struct works (changing from a
	// value receiver to a pointer receiver) to do all the good
	// practices, as well as introducing locking to check for a
	// closed value. I don't care, I'll do it this way.
	func() {
		defer func() {
			recover()
		}()
		h.msgBuf <- rec
	}()

	return nil
}

func (h Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Copy self.
	res := h

	// Concatenate old attrs with new ones.
	// Not using append() because that would modify the old struct's slice.
	res.attrs = slices.Concat(res.attrs, attrs)

	return res
}

func (h Handler) WithGroup(name string) slog.Handler {
	// Copy self.
	res := h

	// Copy old attrs.
	res.attrs = slices.Clone(h.attrs)

	// Append prefix.
	res.attrKeyPrefix += name + "."

	return res
}

var _ slog.Handler = (*Handler)(nil)
