package state

import (
	"context"
	"sync"
)

// DirectoryLoader performs directory reads asynchronously.
type DirectoryLoader interface {
	Start(req DirectoryLoadRequest)
	Cancel(token int)
}

// DirectoryLoadRequest describes a directory read to perform.
type DirectoryLoadRequest struct {
	Token    int
	Path     string
	Callback func(DirectoryLoadResult)
}

// DirectoryLoadResult is emitted by DirectoryLoader once the read completes.
type DirectoryLoadResult struct {
	Token   int
	Path    string
	Entries []FileEntry
	Err     error
}

// NewAsyncDirectoryLoader constructs the default goroutine-based loader.
func NewAsyncDirectoryLoader() DirectoryLoader {
	return &asyncDirectoryLoader{
		jobs: make(map[int]context.CancelFunc),
	}
}

type asyncDirectoryLoader struct {
	mu   sync.Mutex
	jobs map[int]context.CancelFunc
}

func (l *asyncDirectoryLoader) Start(req DirectoryLoadRequest) {
	if req.Token == 0 || req.Path == "" || req.Callback == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	l.mu.Lock()
	l.jobs[req.Token] = cancel
	l.mu.Unlock()

	go func() {
		defer func() {
			l.mu.Lock()
			delete(l.jobs, req.Token)
			l.mu.Unlock()
		}()

		entries, err := readDirectoryEntries(req.Path)

		select {
		case <-ctx.Done():
			return
		default:
		}

		req.Callback(DirectoryLoadResult{
			Token:   req.Token,
			Path:    req.Path,
			Entries: entries,
			Err:     err,
		})
	}()
}

func (l *asyncDirectoryLoader) Cancel(token int) {
	l.mu.Lock()
	if cancel, ok := l.jobs[token]; ok {
		cancel()
		delete(l.jobs, token)
	}
	l.mu.Unlock()
}
