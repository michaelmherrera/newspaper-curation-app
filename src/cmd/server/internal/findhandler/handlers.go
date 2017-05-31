package findhandler

import (
	"cmd/server/internal/responder"
	"legacyfinder"
	"net/http"
	"path"
	"web/tmpl"

	"github.com/gorilla/mux"
)

var basePath string
var watcher *legacyfinder.Watcher
var Layout *tmpl.TRoot
var HomeTmpl *tmpl.Template
var SearchFormAction string

// Setup sets up all the handler-specific routing, templates, etc
func Setup(r *mux.Router, webPath string, w *legacyfinder.Watcher) {
	basePath = webPath
	SearchFormAction = path.Join(basePath, "results")
	var s = r.PathPrefix(basePath).Subrouter()
	s.Path("").Handler(responder.CanSearchIssues(HomeHandler))
	s.Path("/results").Handler(responder.CanSearchIssues(SearchResultsHandler))

	watcher = w
	Layout = responder.Layout.Clone()
	Layout.Path = path.Join(Layout.Path, "find")
	HomeTmpl = Layout.MustBuild("home.go.html")
}

// rsp returns a Response pre-populated with data vars specific to this handler
func rsp(w http.ResponseWriter, req *http.Request) *responder.Responder {
	var r = responder.Response(w, req)
	r.Vars.Data["SearchFormAction"] = SearchFormAction
	return r
}

// assignUniqueTitles puts a title list into the given responder's data
func assignUniqueTitles(r *responder.Responder) {
	var titles = watcher.IssueFinder().Titles.Unique()
	titles.SortByName()
	r.Vars.Data["Titles"] = titles
}

// HomeHandler shows the search form
func HomeHandler(w http.ResponseWriter, req *http.Request) {
	var r = rsp(w, req)
	assignUniqueTitles(r)
	r.Vars.Title = "Find issues"
	r.Render(HomeTmpl)
}

func SearchResultsHandler(w http.ResponseWriter, req *http.Request) {
}
