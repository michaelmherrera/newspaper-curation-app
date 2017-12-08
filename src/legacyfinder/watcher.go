package legacyfinder

import (
	"config"

	"issuefinder"
	"issuesearch"

	"os"
	"path/filepath"
	"schema"
	"strings"
	"sync"
	"time"

	"github.com/uoregon-libraries/gopkg/fileutil"
	"github.com/uoregon-libraries/gopkg/logger"
)

// A Watcher wraps the local Finder to provide a long-running issue watcher
// which scans issue directories and the live site at regular intervals
type Watcher struct {
	sync.RWMutex
	*Finder
	lookup          *issuesearch.Lookup
	status          watcherStatus
	lastFullRefresh time.Time
	done            chan bool
}

type watcherStatus int

const (
	running watcherStatus = 1 << iota
	refreshing
	finished
)

func (ws watcherStatus) String() string {
	var str []string
	if ws&running != 0 {
		str = append(str, "running")
	}
	if ws&refreshing != 0 {
		str = append(str, "refreshing")
	}
	if ws&finished != 0 {
		str = append(str, "finished")
	}
	return strings.Join(str, "/")
}

// NewWatcher creates a legacy issue Watcher.  Watch() must be called to begin
// looking for issues.
func NewWatcher(conf *config.Config, webroot string, cachePath string) *Watcher {
	// We want our first load to reuse the existing cache if available, because
	// an app restart usually happens very shortly after a crash / server reboot
	return &Watcher{
		Finder:          &Finder{finder: issuefinder.New(), config: conf, webroot: webroot, tempdir: cachePath},
		lastFullRefresh: time.Now(),
		done:            make(chan bool),
	}
}

// IssueFinder returns the underlying issuefinder.Finder.  This will be nil
// until the initial scan is complete
func (w *Watcher) IssueFinder() *issuefinder.Finder {
	return w.Finder.finder
}

// Watch loops forever, refreshing the data in the underlying Finder every so
// often.  The refreshing happens on a new issuefinder.Finder which then
// replaces the current finder data, preventing slow searches from holding up
// read access.
func (w *Watcher) Watch(interval time.Duration) {
	w.Lock()

	// If a cache file is available, use it, but we'll still be refreshing data
	// immediately; this just gets the watcher up and running more quickly
	var cacheFile = filepath.Join(w.tempdir, "finder.cache")
	if cacheFile != "" && fileutil.Exists(cacheFile) {
		var finder, err = issuefinder.Deserialize(cacheFile)
		if err != nil {
			logger.Fatalf("Unable to deserialize the cache file %#v: %s", cacheFile, err)
		}
		w.finder = finder
		w.lookup = issuesearch.NewLookup()
		w.lookup.Populate(w.Finder.finder.Issues)
	}

	if w.status&running != 0 {
		logger.Warnf("Trying to watch issues on an in-progress finder (status: %s)", w.status)
		w.Unlock()
		return
	}
	w.status |= running
	w.Unlock()

	var lastRefresh time.Time
	for {
		if time.Since(lastRefresh) > interval {
			w.refresh()
			lastRefresh = time.Now()
			var err = w.finder.Serialize(cacheFile)
			if err != nil {
				logger.Warnf("Unable to cache to %#v: %s", cacheFile, err)
			}
		}
		time.Sleep(time.Second * 1)

		// If loop should no longer be running, we send the done signal and exit
		w.RLock()
		var stopped = (w.status&running == 0)
		w.RUnlock()
		if stopped {
			w.done <- true
			return
		}
	}
}

// Stop signals the watch loop to stop running, allowing for cleanup to happen safely
func (w *Watcher) Stop() {
	w.Lock()
	if w.status&running == 0 {
		w.Unlock()
		return
	}
	w.status &= ^running
	w.Unlock()

	// Wait for the signal that it's done, then clean up
	w.Lock()
	_ = <-w.done
	w.status = finished
	w.cleanupTempDir()
	w.Unlock()
}

// cleanupTempDir removes the httpcache temp dir files and subdirectories.
// This must have a lock to be used safely.
func (w *Watcher) cleanupTempDir() {
	if w.tempdir == "" {
		return
	}
	var err = os.RemoveAll(w.tempdir)
	if err != nil {
		logger.Errorf("Unable to remove legacyfinder.Watcher's temp dir %#v: %s", w.tempdir, err)
	}
	w.tempdir = ""
}

// makeTempDir creates the temporary directory for httpcache to use.  This does
// nothing if a temporary directory already exists.
func (w *Watcher) makeTempDir() {
	var err = os.MkdirAll(w.tempdir, 0700)
	if err != nil {
		logger.Errorf("Unable to create legacyfinder.Watcher's temp dir: %s", err)
	}
}

// refresh runs the searchers and replaces the underlying issuefinder.Finder.
// Every week, it forces a full refresh of web data as well.
func (w *Watcher) refresh() {
	logger.Debugf("Refreshing issue data")
	w.Lock()

	// Safety: is run off already?  This can only happen if stop was called just
	// as this was about to be called, but it's still better to be safe.
	if w.status&running == 0 {
		logger.Errorf("Trying to refresh a stopped legacyfinder")
		w.Unlock()
		return
	}

	// Don't try to run multiple refreshes!
	if w.status&refreshing != 0 {
		logger.Errorf("Trying to double-refresh a legacyfinder")
		w.Unlock()
		return
	}

	w.status |= refreshing
	w.Unlock()

	// Every week, we force a full web refresh
	var tempdir string
	if time.Since(w.lastFullRefresh) > time.Hour*24*7 {
		logger.Debugf("Purging cache and reindexing all data from scratch")

		// We don't want to delete tempdir when it's a routine cleaning!  TODO: make this way less hacky
		tempdir = w.tempdir
		w.cleanupTempDir()
		w.tempdir = tempdir
		w.lastFullRefresh = time.Now()
	}

	// This won't do anything if we already have a temp dir
	w.makeTempDir()

	// Now actually run the finder and replace it; during this process it's safe
	// for other stuff to happen
	var finder, err = w.FindIssues()

	// This is supposed to happen in the background, so an error can only be
	// reported; we can't do much else....
	if err != nil {
		w.Lock()
		w.status &= ^refreshing
		w.Unlock()
		logger.Errorf("Unable to refresh legacyfinder: %s", err)
		return
	}

	// Create a new lookup using the new finder's data
	var lookup = issuesearch.NewLookup()
	lookup.Populate(finder.Issues)

	// Re-acquire lock to swap out the finder and lookup, then update status
	w.Lock()
	w.finder = finder
	w.lookup = lookup
	w.status &= ^refreshing
	w.Unlock()

	logger.Debugf("Issue data refreshed")
}

// LookupIssues returns a list of schema Issues for the give search key
func (w *Watcher) LookupIssues(key *issuesearch.Key) []*schema.Issue {
	return w.lookup.Issues(key)
}
