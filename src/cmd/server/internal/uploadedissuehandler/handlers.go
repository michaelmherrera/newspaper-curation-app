package uploadedissuehandler

import (
	"apperr"
	"cmd/server/internal/responder"
	"config"
	"encoding/base64"
	"fmt"
	"issuewatcher"

	"net/http"
	"path"
	"web/tmpl"

	"github.com/gorilla/mux"
)

var (
	searcher *Searcher
	watcher  *issuewatcher.Watcher
	conf     *config.Config

	// basePath is the path to the main uploaded issues page.  Subpages all start with this path.
	basePath string

	// Layout is the base template, cloned from the responder's layout, from
	// which all subpages are built
	Layout *tmpl.TRoot

	// TitleList renders the uploaded issues landing page
	TitleList *tmpl.Template

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
	s.Path("/{title}").Handler(canView(TitleHandler))
	s.Path("/{title}/{issue}").Handler(canView(IssueHandler))
	s.Path("/{title}/{issue}/workflow/{action}").Methods("POST").Handler(canModify(IssueWorkflowHandler))
	s.Path("/{title}/{issue}/{filename}").Handler(canView(FileHandler))

	searcher = newSearcher(c)
	Layout = responder.Layout.Clone()

	// Add custom functions we need for display of schema object errors
	Layout.Funcs(tmpl.FuncMap{"ErrorHTML": errorHTML, "ErrorListHTML": errorListHTML})

	Layout.Path = path.Join(Layout.Path, "uploadedissues")
	TitleList = Layout.MustBuild("title-list.go.html")
	IssueTmpl = Layout.MustBuild("issue.go.html")
	TitleTmpl = Layout.MustBuild("title.go.html")
}

// HomeHandler spits out the title list
func HomeHandler(w http.ResponseWriter, req *http.Request) {
	var r = getResponder(w, req)
	r.Vars.Title = "Uploaded Issues"
	if searcher.Ready() {
		r.Vars.Data["OtherErrors"] = searcher.TopErrors()
	} else {
		r.Vars.Data["OtherErrors"] = []string{}
	}
	r.Render(TitleList)
}

// TitleHandler prints a list of issues for a given title
func TitleHandler(w http.ResponseWriter, req *http.Request) {
	var r = getResponder(w, req)
	if r.err != nil {
		r.Render(nil)
		return
	}
	r.Vars.Title = fmt.Sprintf("%s - %s", r.title.Name, r.title.Type)
	r.Render(TitleTmpl)
}

// IssueHandler prints a list of pages for a given issue
func IssueHandler(w http.ResponseWriter, req *http.Request) {
	var r = getResponder(w, req)
	if r.err != nil {
		r.Render(nil)
		return
	}
	r.Vars.Title = fmt.Sprintf("%s, issue %s - %s", r.title.Name, r.issue.RawDate, r.title.Type)
	r.Render(IssueTmpl)
}

// IssueWorkflowHandler handles setting up the issue move job
func IssueWorkflowHandler(w http.ResponseWriter, req *http.Request) {
	// Since we have real logic in this handler, we want to bail if we already
	// know there are errors
	var r = getResponder(w, req)
	if r.err != nil {
		r.Render(nil)
		return
	}

	switch r.vars["action"] {
	case "queue":
		var err = r.issue.Queue()
		var cname, msg string
		if err == nil {
			cname = "Info"
			msg = "Issue queued successfully"
			searcher.RemoveIssue(r.issue)
		} else {
			cname = "Alert"
			msg = encodedError(err)
		}

		r.Audit("queue", fmt.Sprintf("Issue from %q, success: %#v", r.issue.Location, err == nil))
		http.SetCookie(w, &http.Cookie{Name: cname, Value: msg, Path: "/"})
		http.Redirect(w, req, TitlePath(r.issue.Title.Slug), http.StatusFound)

	default:
		r.Error(http.StatusBadRequest, "")
	}
}

// encodedError creates a base64 alert for errors to be displayed
func encodedError(err apperr.Error) string {
	return "base64" + base64.StdEncoding.EncodeToString([]byte(err.Message()))
}
