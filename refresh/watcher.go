package refresh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/markbates/refresh/filenotify"
)

type Watcher struct {
	MainWatcher       filenotify.FileWatcher
	LivereloadWatcher filenotify.FileWatcher
	*Manager
	context context.Context
}

func NewWatcher(r *Manager) *Watcher {
	var (
		lr, main filenotify.FileWatcher
	)

	if r.ForcePolling {
		main = filenotify.NewPollingWatcher()
	} else {
		main, _ = filenotify.NewEventWatcher()
	}

	lr, _ = filenotify.NewEventWatcher()

	return &Watcher{
		MainWatcher:       main,
		LivereloadWatcher: lr,
		Manager:           r,
		context:           r.context,
	}
}

func (w *Watcher) Start() {
	go func() {
		for {
			err := filepath.Walk(w.AppRoot, func(path string, info os.FileInfo, err error) error {
				if w.isLivereloaderEnable() {
					for _, v := range w.Livereload.IncludedFolders {
						w.IgnoredFolders = append(w.IgnoredFolders, v)
						w.LivereloadWatcher.Add(v)
					}
				}

				if info == nil {
					w.cancelFunc()
					return errors.New("nil directory")
				}

				if info.IsDir() {
					if strings.HasPrefix(filepath.Base(path), "_") {
						return filepath.SkipDir
					}
					if len(path) > 1 && strings.HasPrefix(filepath.Base(path), ".") || w.isIgnoredFolder(path) {
						return filepath.SkipDir
					}
				}

				if w.isWatchedFile(path) {
					w.MainWatcher.Add(path)
				}

				return nil
			})

			if err != nil {
				w.context.Done()
				break
			}
			// sweep for new files every 1 second
			time.Sleep(1 * time.Second)
		}
	}()
}

func (w Watcher) isIgnoredFolder(path string) bool {
	paths := strings.Split(path, "/")
	if len(paths) <= 0 {
		return false
	}

	for _, e := range w.IgnoredFolders {
		if strings.TrimSpace(e) == paths[0] {
			return true
		}
	}
	return false
}

func (w Watcher) isWatchedFile(path string) bool {
	ext := filepath.Ext(path)

	for _, e := range w.IncludedExtensions {
		if strings.TrimSpace(e) == ext {
			return true
		}
	}

	return false
}
