package filestore

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

// MemoryStore is an in-memory Store for tests. It is not safe for concurrent
// use (test harnesses typically run one test at a time per instance).
type MemoryStore struct {
	objects map[string][]byte
	types   map[string]string
}

// NewMemoryStore builds an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		objects: make(map[string][]byte),
		types:   make(map[string]string),
	}
}

// Put stores an object in memory.
func (m *MemoryStore) Put(_ context.Context, key, contentType string, body io.Reader) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	m.objects[key] = data
	m.types[key] = contentType
	return nil
}

// Get returns a stored object or an error if absent.
func (m *MemoryStore) Get(_ context.Context, key string) (*Object, error) {
	data, ok := m.objects[key]
	if !ok {
		return nil, fmt.Errorf("object %q not found", key)
	}
	return &Object{
		Body:        io.NopCloser(bytes.NewReader(data)),
		ContentType: m.types[key],
		Size:        int64(len(data)),
	}, nil
}

// Delete removes an object from memory. Missing objects do not error.
func (m *MemoryStore) Delete(_ context.Context, key string) error {
	delete(m.objects, key)
	delete(m.types, key)
	return nil
}
