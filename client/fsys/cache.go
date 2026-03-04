package fsys

import (
	"context"
	"sync"
	"time"

	"friendnet.org/common"
	pb "friendnet.org/protocol/pb/v1"
)

type metaCacheEntry struct {
	meta  *pb.MsgFileMeta
	expTs time.Time
}

type metaCacheKey struct {
	prefix string
	path   common.ProtoPath
}

// MetaCache caches file metadata for paths.
// Key prefixes are prepended before the paths to allow for multiple different users of the cache.
type MetaCache struct {
	mu       sync.RWMutex
	isClosed bool

	ctx       context.Context
	ctxCancel context.CancelFunc

	cache      map[metaCacheKey]metaCacheEntry
	entryTtl   time.Duration
	gcInterval time.Duration
}

// NewMetaCache creates a new MetaCache with the specified entry TTL and garbage collection interval.
// Entries will be inaccessible if expired even if the GC hasn't run to remove them yet.
func NewMetaCache(entryTtl time.Duration, gcInterval time.Duration) *MetaCache {
	ctx, cancel := context.WithCancel(context.Background())

	c := &MetaCache{
		ctx:       ctx,
		ctxCancel: cancel,

		cache:      make(map[metaCacheKey]metaCacheEntry),
		entryTtl:   entryTtl,
		gcInterval: gcInterval,
	}

	go c.gc()

	return c
}

func (c *MetaCache) Close() error {
	c.mu.Lock()
	if c.isClosed {
		c.mu.Unlock()
		return nil
	}
	c.isClosed = true
	c.cache = nil
	c.mu.Unlock()

	c.ctxCancel()
	return nil
}

func (c *MetaCache) gc() {
	ticker := time.NewTicker(c.gcInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			for key, entry := range c.cache {
				if entry.expTs.Before(time.Now()) {
					delete(c.cache, key)
				}
			}
			c.mu.Unlock()
		}
	}
}

// Get returns a cached file metadata entry for a path.
// Returns false if the entry does not exist, has expired, or if the cache is closed.
func (c *MetaCache) Get(keyPrefix string, path common.ProtoPath) (*pb.MsgFileMeta, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.isClosed {
		return nil, false
	}

	key := metaCacheKey{
		prefix: keyPrefix,
		path:   path,
	}

	entry, has := c.cache[key]
	if !has {
		return nil, false
	}

	if entry.expTs.Before(time.Now()) {
		return nil, false
	}

	return entry.meta, true
}

// Set adds a file metadata entry to the cache.
// If there is a previous entry, it overwrites it.
// No-op if the cache is closed.
func (c *MetaCache) Set(keyPrefix string, path common.ProtoPath, meta *pb.MsgFileMeta) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isClosed {
		return
	}

	key := metaCacheKey{
		prefix: keyPrefix,
		path:   path,
	}

	c.cache[key] = metaCacheEntry{
		meta:  meta,
		expTs: time.Now().Add(c.entryTtl),
	}
}
