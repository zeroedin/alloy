package cmd

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/server"
)

type watcherDispatchFn func(events []server.ChangeEvent, scope server.RebuildScope)

func startWatcher(cfg *config.Config, srv *server.Server, dispatch watcherDispatchFn) *fsnotify.Watcher {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("warning: file watcher unavailable: %v", err)
		return nil
	}

	watchDirs := server.WatchDirs(cfg)
	for _, dir := range watchDirs {
		absDir := dir
		if cfg.ProjectRoot != "" && !filepath.IsAbs(dir) {
			absDir = filepath.Join(cfg.ProjectRoot, dir)
		}
		if info, err := os.Stat(absDir); err == nil && info.IsDir() {
			if err := addRecursiveWatch(watcher, absDir); err != nil {
				log.Printf("warning: watching %s: %v", dir, err)
			}
		}
	}

	debouncer := server.NewDebouncer(
		time.Duration(srv.DebounceInterval())*time.Millisecond,
		10,
	)

	go func() {
		var pending []server.ChangeEvent
		timer := time.NewTimer(time.Duration(srv.DebounceInterval()) * time.Millisecond)
		timer.Stop()

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
					continue
				}

				relPath := event.Name
				if cfg.ProjectRoot != "" {
					if r, err := filepath.Rel(cfg.ProjectRoot, event.Name); err == nil && !strings.HasPrefix(r, "..") {
						relPath = r
					}
				}

				changeType := server.ClassifyChange(relPath, cfg)
				pending = append(pending, server.ChangeEvent{
					Path:       relPath,
					ChangeType: changeType,
				})

				if event.Op&fsnotify.Create != 0 {
					if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
						if err := addRecursiveWatch(watcher, event.Name); err != nil {
							log.Printf("warning: watching %s: %v", event.Name, err)
						}
					}
				}

				timer.Reset(time.Duration(srv.DebounceInterval()) * time.Millisecond)

			case <-timer.C:
				if len(pending) == 0 {
					continue
				}

				events := pending
				pending = nil

				events, scope := debouncer.Debounce(events)
				dispatch(events, scope)

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("watcher error: %v", err)
			}
		}
	}()

	return watcher
}

func addRecursiveWatch(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
}
