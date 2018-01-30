package main

import (
	"cmd/server/internal/responder"
	"cmd/server/internal/settings"
	"cmd/server/internal/sftphandler"
	"cmd/server/internal/workflowhandler"
	"config"
	"db"
	"path/filepath"

	"fmt"
	"issuewatcher"

	"net/http"
	"net/url"
	"os"
	"path"
	"time"
	"user"
	"web/webutil"

	"github.com/gorilla/mux"
	"github.com/jessevdk/go-flags"
	"github.com/uoregon-libraries/gopkg/logger"
)

var opts struct {
	ParentWebroot string `long:"parent-webroot" description:"The base path to the parent app" required:"true"`
	ConfigFile    string `short:"c" long:"config" description:"path to P2C config file" required:"true"`
	Debug         bool   `long:"debug" description:"Enables debug mode for testing different users"`
}

// Conf stores the configuration data read from the legacy Python settings
var Conf *config.Config

func getConf() {
	var p = flags.NewParser(&opts, flags.HelpFlag|flags.PassDoubleDash)
	var _, err = p.Parse()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n\n", err)
		p.WriteHelp(os.Stderr)
		os.Exit(1)
	}

	Conf, err = config.Parse(opts.ConfigFile)
	if err != nil {
		logger.Fatalf("Config error: %s", err)
	}

	err = db.Connect(Conf.DatabaseConnect)
	if err != nil {
		logger.Fatalf("Error trying to connect to database: %s", err)
	}
	user.DB = db.DB

	// We can ignore the error here because the config magic already verified
	// that the URL was valid
	var u, _ = url.Parse(Conf.Webroot)
	webutil.Webroot = u.Path
	webutil.ParentWebroot = opts.ParentWebroot
	webutil.WorkflowPath = Conf.WorkflowPath
	webutil.IIIFBaseURL = Conf.IIIFBaseURL

	if opts.Debug == true {
		logger.Warnf("Debug mode has been enabled")
		settings.DEBUG = true
		db.Debug = true
	}

	responder.InitRootTemplate(filepath.Join(Conf.AppRoot, "templates"))
}

func makeRedirect(dest string, code int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest, code)
	})
}

func startServer() {
	var r = mux.NewRouter()
	var hp = webutil.HomePath()

	// Make sure homepath/ isn't considered the canonical path
	r.Handle(hp+"/", makeRedirect(hp, http.StatusMovedPermanently))

	// The static handler doesn't check permissions.  Right now this is okay, as
	// what we serve isn't valuable beyond page layout, but this may warrant a
	// fileserver clone + rewrite.
	var fileServer = http.FileServer(http.Dir(filepath.Join(Conf.AppRoot, "static")))
	var staticPrefix = path.Join(hp, "static")
	r.NewRoute().PathPrefix(staticPrefix).Handler(http.StripPrefix(staticPrefix, fileServer))

	var watcher = issuewatcher.New(Conf)
	go watcher.Watch(5 * time.Minute)
	sftphandler.Setup(r, path.Join(hp, "sftp"), Conf, watcher)
	workflowhandler.Setup(r, path.Join(hp, "workflow"), Conf, watcher)

	var waited, lastWaited int
	for watcher.IssueFinder().Issues == nil {
		if waited == 5 {
			logger.Infof("Waiting for initial issue scan to complete.  This can take " +
				"several minutes if the issues haven't been scanned in a while.  If this " +
				"is the first time scanning the live site, expect 10 minutes or more to " +
				"build the web JSON cache.")
		} else if waited/30 > lastWaited {
			logger.Infof("Still waiting...")
			lastWaited = waited / 30
		}
		waited++
		time.Sleep(1 * time.Second)
	}

	http.Handle("/", nocache(logMiddleware(r)))

	logger.Infof("Listening on %s", Conf.BindAddress)
	if err := http.ListenAndServe(Conf.BindAddress, nil); err != nil {
		logger.Fatalf("Error starting listener: %s", err)
	}
}

func main() {
	getConf()
	startServer()
}
