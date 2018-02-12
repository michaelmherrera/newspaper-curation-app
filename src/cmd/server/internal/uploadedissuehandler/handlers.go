package uploadedissuehandler

import (
	"cmd/server/internal/responder"
	"config"
	"fmt"
	"issuewatcher"

	"net/http"
	"path"
	"web/tmpl"

	"github.com/gorilla/mux"
	"github.com/uoregon-libraries/gopkg/logger"
)

var (
	sftpSearcher *SFTPSearcher
	watcher      *issuewatcher.Watcher
	conf         *config.Config

	// basePath is the path to the main uploaded issues page.  Subpages all start with this path.
	basePath string

	// Layout is the base template, cloned from the responder's layout, from
	// which all subpages are built
	Layout *tmpl.TRoot

	// HomeTmpl renders the uploaded issues landing page
	HomeTmpl *tmpl.Template

	// TitleTmpl renders the list of issues and a summary of errors for a given title
	TitleTmpl *tmpl.Template

	// IssueTmpl renders the list of PDFs and errors in a given issue
	IssueTmpl *tmpl.Template
)

// Setup sets up all the routing rules and other configuration
func Setup(r *mux.Router, baseWebPath string, c *config.Config, w *issuewatcher.Watcher) {
	conf = c
	watcher = w
	basePath = baseWebPath
	var s = r.PathPrefix(basePath).Subrouter()
	s.Path("").Handler(canView(HomeHandler))
	s.Path("/{lccn}").Handler(canView(TitleHandler))
	s.Path("/{lccn}/{issue}").Handler(canView(IssueHandler))
	s.Path("/{lccn}/{issue}/workflow/{action}").Methods("POST").Handler(canModify(IssueWorkflowHandler))
	s.Path("/{lccn}/{issue}/{filename}").Handler(canView(PDFFileHandler))

	sftpSearcher = newSFTPSearcher(conf.MasterPDFUploadPath)
	Layout = responder.Layout.Clone()
	Layout.Path = path.Join(Layout.Path, "uploadedissues")
	HomeTmpl = Layout.MustBuild("home.go.html")
	IssueTmpl = Layout.MustBuild("issue.go.html")
	TitleTmpl = Layout.MustBuild("title.go.html")
}

// HomeHandler spits out the title list
func HomeHandler(w http.ResponseWriter, req *http.Request) {
	var r = getResponder(w, req)
	logger.Debugf("There are %d titles", len(r.sftpTitles))
	r.Vars.Title = "All Uploaded Issues' Titles"
	r.Render(HomeTmpl)
}

// TitleHandler prints a list of issues for a given title
func TitleHandler(w http.ResponseWriter, req *http.Request) {
	var r = getResponder(w, req)
	r.Vars.Title = "Issues for " + r.title.Name
	r.Render(TitleTmpl)
}

// IssueHandler prints a list of pages for a given issue
func IssueHandler(w http.ResponseWriter, req *http.Request) {
	var r = getResponder(w, req)
	r.Vars.Title = fmt.Sprintf("Files for %s, issue %s", r.title.Name, r.issue.Date.Format("2006-01-02"))
	r.Render(IssueTmpl)
}

// IssueWorkflowHandler handles setting up the sftp move job
func IssueWorkflowHandler(w http.ResponseWriter, req *http.Request) {
	// Since we have real logic in this handler, we want to bail if we already
	// know there are errors
	var r = getResponder(w, req)
	if r.err != nil {
		return
	}

	switch r.vars["action"] {
	case "queue":
		var ok, msg = queueSFTPIssueMove(r.issue)
		var cname = "Info"
		if !ok {
			cname = "Alert"
		}

		r.Audit("sftp-queue", fmt.Sprintf("Issue %q, success: %#v", r.issue.Key(), ok))
		http.SetCookie(w, &http.Cookie{Name: cname, Value: msg, Path: "/"})
		http.Redirect(w, req, TitlePath(r.issue.Title.Slug), http.StatusFound)

	default:
		r.Error(http.StatusBadRequest, "")
	}
}
