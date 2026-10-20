// Package index persists history and queries file co-change through postings.
package index

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nicovegasr/nib-git/internal/history"
	bolt "go.etcd.io/bbolt"
)

const schemaVersion = 1

var bucketNames = []string{"meta", "commits", "oids", "files", "paths", "touches"}

// ErrRewrite requires an explicit rebuild instead of mixing divergent histories.
var ErrRewrite = errors.New("history diverged from indexed HEAD; run nib index --rebuild")

// ErrStale prevents interpreting an old snapshot as the current repository.
var ErrStale = errors.New("index is stale; run nib index")

// Source supplies an immutable snapshot and streams unseen history.
type Source interface {
	Identity() string
	Revision() string
	IsAncestor(context.Context, string) (bool, error)
	Walk(context.Context, string, func(history.Commit) error) error
}

// Metadata identifies the complete, committed index snapshot.
type Metadata struct {
	Version  int    `json:"schema_version"`
	Identity string `json:"repository"`
	Head     string `json:"head"`
	Commits  int    `json:"commits"`
	Files    int    `json:"files"`
}

// Result reports how much work an update actually performed.
type Result struct {
	Metadata
	Added int `json:"added_commits"`
}

// Store owns a database handle. Call Close before opening another writer.
type Store struct{ db *bolt.DB }

// Open uses a bounded lock wait and never creates an index for a read query.
func Open(path string, readOnly bool) (*Store, error) {
	if !readOnly {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{ReadOnly: readOnly, Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("open history index (run nib index if absent): %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the database and its lock.
func (s *Store) Close() error { return s.db.Close() }

func bucket(tx *bolt.Tx, name string) *bolt.Bucket { return tx.Bucket([]byte(name)) }
func key(id uint64) []byte                         { b := make([]byte, 8); binary.BigEndian.PutUint64(b, id); return b }

func metadata(tx *bolt.Tx) (Metadata, error) {
	var m Metadata
	b := bucket(tx, "meta")
	if b == nil {
		return m, nil
	}
	data := b.Get([]byte("snapshot"))
	if data == nil {
		return m, nil
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	if m.Version != schemaVersion {
		return m, errors.New("unsupported index schema; run nib index --rebuild")
	}
	return m, nil
}

// Metadata reads only a completed snapshot.
func (s *Store) Metadata() (Metadata, error) {
	var m Metadata
	err := s.db.View(func(tx *bolt.Tx) error { var err error; m, err = metadata(tx); return err })
	return m, err
}

// Check validates identity and freshness before a query.
func (s *Store) Check(identity, head string) error {
	m, err := s.Metadata()
	if err != nil {
		return err
	}
	if m.Identity != identity {
		return errors.New("index belongs to another repository; run nib index --rebuild")
	}
	if m.Head != head {
		return ErrStale
	}
	return nil
}

type storedChange struct {
	FileID       uint64 `json:"file_id"`
	PreviousPath string `json:"previous_path,omitempty"`
	Kind         string `json:"kind"`
	Added        int64  `json:"added"`
	Deleted      int64  `json:"deleted"`
	Binary       bool   `json:"binary,omitempty"`
}

type storedCommit struct {
	history.Commit
	ID    uint64         `json:"id"`
	Files []storedChange `json:"files"`
}

func initialize(tx *bolt.Tx, rebuild bool) error {
	for _, name := range bucketNames {
		if rebuild && bucket(tx, name) != nil {
			if err := tx.DeleteBucket([]byte(name)); err != nil {
				return err
			}
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
			return err
		}
	}
	return nil
}

// Update atomically imports new reachable commits or replaces the whole index.
func (s *Store) Update(ctx context.Context, src Source, rebuild bool) (Result, error) {
	var result Result
	err := s.db.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		m, err := metadata(tx)
		if err != nil && !rebuild {
			return err
		}
		if rebuild {
			m = Metadata{}
		}
		if err = checkAdvance(ctx, src, m); err != nil {
			return err
		}
		if err = initialize(tx, rebuild); err != nil {
			return err
		}
		if m.Head != src.Revision() {
			err = src.Walk(ctx, m.Head, func(c history.Commit) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				added, err := insert(tx, c)
				if added {
					result.Added++
				}
				return err
			})
			if err != nil {
				return err
			}
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		m = Metadata{Version: schemaVersion, Identity: src.Identity(), Head: src.Revision(), Commits: int(bucket(tx, "commits").Sequence()), Files: int(bucket(tx, "files").Sequence())}
		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		result.Metadata = m
		return bucket(tx, "meta").Put([]byte("snapshot"), data)
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func checkAdvance(ctx context.Context, src Source, m Metadata) error {
	if m.Identity != "" && m.Identity != src.Identity() {
		return errors.New("repository identity changed; run nib index --rebuild")
	}
	if m.Head == "" || m.Head == src.Revision() {
		return nil
	}
	ok, err := src.IsAncestor(ctx, m.Head)
	if err != nil {
		return fmt.Errorf("check indexed ancestor (rebuild if history was removed): %w", err)
	}
	if !ok {
		return ErrRewrite
	}
	return nil
}

func fileID(tx *bolt.Tx, path string) (uint64, error) {
	files := bucket(tx, "files")
	if data := files.Get([]byte(path)); data != nil {
		return binary.BigEndian.Uint64(data), nil
	}
	id, err := files.NextSequence()
	if err != nil {
		return 0, err
	}
	if err = files.Put([]byte(path), key(id)); err != nil {
		return 0, err
	}
	return id, bucket(tx, "paths").Put(key(id), []byte(path))
}

func insert(tx *bolt.Tx, c history.Commit) (bool, error) {
	oids := bucket(tx, "oids")
	if oids.Get([]byte(c.OID)) != nil {
		return false, nil
	}
	commits := bucket(tx, "commits")
	id, err := commits.NextSequence()
	if err != nil {
		return false, err
	}
	stored := storedCommit{Commit: c, ID: id}
	stored.Changes = nil
	for _, change := range c.Changes {
		fid, err := fileID(tx, change.Path)
		if err != nil {
			return false, err
		}
		stored.Files = append(stored.Files, storedChange{FileID: fid, PreviousPath: change.PreviousPath, Kind: change.Kind, Added: change.Added, Deleted: change.Deleted, Binary: change.Binary})
		if err = bucket(tx, "touches").Put(append(key(fid), key(id)...), []byte{1}); err != nil {
			return false, err
		}
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return false, err
	}
	if err = commits.Put(key(id), data); err != nil {
		return false, err
	}
	return true, oids.Put([]byte(c.OID), key(id))
}

func readCommit(tx *bolt.Tx, id []byte) (storedCommit, error) {
	var c storedCommit
	err := json.Unmarshal(bucket(tx, "commits").Get(id), &c)
	return c, err
}

func postings(tx *bolt.Tx, fid []byte, visit func(storedCommit) error) error {
	cursor := bucket(tx, "touches").Cursor()
	for k, _ := cursor.Seek(fid); k != nil && bytes.HasPrefix(k, fid); k, _ = cursor.Next() {
		if len(k) != 16 {
			return errors.New("invalid file posting in index")
		}
		c, err := readCommit(tx, k[8:])
		if err != nil {
			return err
		}
		if err = visit(c); err != nil {
			return err
		}
	}
	return nil
}
