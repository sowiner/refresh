package refresh

import (
	"context"
	"fmt"
	"log"

	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jaschaephraim/lrserver"
)

type Manager struct {
	*Configuration
	ID         string
	Logger     *Logger
	Restart    chan bool
	cancelFunc context.CancelFunc
	context    context.Context
	gil        *sync.Once
	lr         *lrserver.Server
}

func New(c *Configuration) *Manager {
	return NewWithContext(c, context.Background())
}

func NewWithContext(c *Configuration, ctx context.Context) *Manager {
	ctx, cancelFunc := context.WithCancel(ctx)
	m := &Manager{
		Configuration: c,
		ID:            ID(),
		Logger:        NewLogger(c),
		Restart:       make(chan bool),
		cancelFunc:    cancelFunc,
		context:       ctx,
		gil:           &sync.Once{},
	}
	return m
}

func (r *Manager) Start() error {
	w := NewWatcher(r)
	w.Start()
	r.startLivereload()

	go r.build(fsnotify.Event{Name: ":start:"})
	if !r.Debug {
		go func() {
		OuterLoop:
			for {
				select {
				case event := <-w.MainWatcher.Events():
					if event.Op != fsnotify.Chmod {
						go r.build(event)
					}
					w.MainWatcher.Remove(event.Name)
					w.MainWatcher.Add(event.Name)
				case event := <-w.LivereloadWatcher.Events():
					go r.Reload(event)
				case <-r.context.Done():
					break OuterLoop
				}
			}
		}()
	}
	go func() {
	OuterLoop:
		for {
			select {
			case err := <-w.MainWatcher.Errors():
				r.Logger.Error(err)
			case err := <-w.LivereloadWatcher.Errors():
				r.Logger.Error(err)
			case <-r.context.Done():
				break OuterLoop
			}
		}
	}()
	r.runner()
	return nil
}

func (r *Manager) build(event fsnotify.Event) {
	r.gil.Do(func() {
		defer func() {
			r.gil = &sync.Once{}
		}()
		r.buildTransaction(func() error {
			// time.Sleep(r.BuildDelay * time.Millisecond)
			now := time.Now()
			r.Logger.Print("Rebuild on: %s", event.Name)

			args := []string{"build", "-v"}
			args = append(args, r.BuildFlags...)
			args = append(args, "-o", r.FullBuildPath(), r.BuildTargetPath)
			cmd := exec.CommandContext(r.context, "go", args...)

			err := r.runAndListen(cmd)
			if err != nil {
				if strings.Contains(err.Error(), "no buildable Go source files") {
					r.cancelFunc()
					log.Fatal(err)
				}
				return err
			}

			tt := time.Since(now)
			r.Logger.Success("Building Completed (PID: %d) (Time: %s)", cmd.Process.Pid, tt)
			r.Restart <- true
			return nil
		})
	})
}

func (r *Manager) buildTransaction(fn func() error) {
	lpath := ErrorLogPath()
	err := fn()
	if err != nil {
		f, _ := os.Create(lpath)
		fmt.Fprint(f, err)
		r.Logger.Error("Error!")
		r.Logger.Error(err)
	} else {
		os.Remove(lpath)
	}
}

func (r *Manager) Reload(event fsnotify.Event) {
	r.gil.Do(func() {
		defer func() {
			r.gil = &sync.Once{}
		}()
		r.runTasks()
		r.lr.Reload(event.Name)
	})
}

func (r *Manager) isLivereloaderEnable() bool {
	if r.Livereload.Enable && r.Livereload.IncludedFolders != nil {
		return true
	}
	return false
}

func (r *Manager) startLivereload() {
	// Create and start LiveReload server
	if !r.isLivereloaderEnable() {
		return
	}
	lr := lrserver.New(lrserver.DefaultName, r.Livereload.Port)
	go lr.ListenAndServe()
	r.lr = lr
}
