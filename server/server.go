// Copyright 2017 Alexey Naidyonov. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.md file.

package server

import (
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/growler/go-slate/slate"
	"github.com/spf13/afero"
)

func monitor(watcher *fsnotify.Watcher, src string, params slate.Params, lock *sync.RWMutex, httpFs **afero.HttpFs) {
	ts := time.Now()
	tm := time.NewTimer(0)
	tm.Stop()
	for {
		select {
		case <-tm.C:
		case event := <-watcher.Events:
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				watcher.Remove(event.Name)
			} else if event.Op&fsnotify.Create == fsnotify.Create {
				watcher.Add(event.Name)
			}
		}
		now := time.Now()
		// totally arbitrary interval to skip bursts of changes
		// (Intellij Goland creates a lot of temporary files for
		// a single save)
		if now.After(ts) {
			ts = now.Add(2 * time.Second)
			fs := afero.NewMemMapFs()
			if err := slate.Slateficate(src, &afero.Afero{Fs: fs}, params); err != nil {
				log.Printf("error processing source: %s", err)
			} else {
				log.Print("documentation sucessfuly updated")
				lock.Lock()
				*httpFs = afero.NewHttpFs(fs)
				lock.Unlock()
			}
			tm.Stop()
		} else {
			tm.Reset(ts.Sub(now))
		}
	}
}

// Serve API documentation from the src location. Monitors changes if requested and
// updates rendered documentation.
func Serve(src string, params slate.Params, addr, tlsCert, tlsKey string, mon bool) error {
	var (
		err    error
		tls    bool
		lock   sync.RWMutex
		httpFs *afero.HttpFs
	)
	log.SetOutput(os.Stderr)
	fs := afero.NewMemMapFs()
	if err := slate.Slateficate(src, &afero.Afero{Fs: fs}, params); err != nil {
		return err
	}
	if tlsCert != "" && tlsKey != "" {
		tls = true
	} else if tlsCert != "" || tlsKey != "" {
		return errors.New("both cert and key must be supplied for HTTPS")
	}
	httpFs = afero.NewHttpFs(fs)
	if mon {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return err
		}
		filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
			if err == nil {
				watcher.Add(path)
			}
			return nil
		})
		defer watcher.Close()
		go monitor(watcher, src, params, &lock, &httpFs)
		http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
			lock.RLock()
			fs := httpFs
			lock.RUnlock()
			if fs == nil {
				http.NotFound(writer, request)
			}
			http.FileServer(fs.Dir("")).ServeHTTP(writer, request)
		})
	} else {
		http.Handle("/", http.FileServer(httpFs.Dir("")))
	}
	if tls {
		err = http.ListenAndServeTLS(addr, tlsCert, tlsKey, nil)
	} else {
		err = http.ListenAndServe(addr, nil)
	}
	return err
}
