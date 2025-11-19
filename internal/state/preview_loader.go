package state

import (
	"context"
	"os"
	"sync"
)

// PreviewLoader performs preview generation asynchronously.
type PreviewLoader interface {
	Start(req PreviewLoadRequest)
	Cancel(token int)
}

// PreviewLoadRequest describes the preview to build.
type PreviewLoadRequest struct {
	Token      int
	Path       string
	HideHidden bool
	Callback   func(PreviewLoadResult)
}

// PreviewLoadResult carries the generated preview or any error.
type PreviewLoadResult struct {
	Token int
	Path  string
	Data  *PreviewData
	Info  os.FileInfo
	Err   error
}

// NewAsyncPreviewLoader constructs the default goroutine-based preview loader.
func NewAsyncPreviewLoader() PreviewLoader {
	return &asyncPreviewLoader{
		jobs: make(map[int]context.CancelFunc),
	}
}

type asyncPreviewLoader struct {
	mu   sync.Mutex
	jobs map[int]context.CancelFunc
}

func (l *asyncPreviewLoader) Start(req PreviewLoadRequest) {
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

		data, info, err := buildPreviewData(req.Path, req.HideHidden)

		select {
		case <-ctx.Done():
			return
		default:
		}

		req.Callback(PreviewLoadResult{
			Token: req.Token,
			Path:  req.Path,
			Data:  data,
			Info:  info,
			Err:   err,
		})
	}()
}

func (l *asyncPreviewLoader) Cancel(token int) {
	l.mu.Lock()
	if cancel, ok := l.jobs[token]; ok {
		cancel()
		delete(l.jobs, token)
	}
	l.mu.Unlock()
}
