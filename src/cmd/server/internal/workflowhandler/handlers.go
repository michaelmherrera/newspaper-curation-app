package workflowhandler

import (
	"cmd/server/internal/responder"
	"config"
	"db"
	"fmt"
	"logger"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
	"web/tmpl"

	"github.com/gorilla/mux"
)

var (
	conf *config.Config

	// basePath is the path to the main workflow page.  Subpages all start with this path.
	basePath string

	// Layout is the base template, cloned from the responder's layout, from
	// which all workflow pages are built
	Layout *tmpl.TRoot

	// DeskTmpl renders the main "workflow desk" page
	DeskTmpl *tmpl.Template

	// MetadataFormTmpl renders the form for entering metadata for an issue
	MetadataFormTmpl *tmpl.Template

	// ReportErrorTmpl renders the form for reporting errors on an issue
	ReportErrorTmpl *tmpl.Template

	// ReviewMetadataTmpl renders the view for reviewing metadata
	ReviewMetadataTmpl *tmpl.Template

	// RejectIssueTmpl renders the view for reporting an issue which is rejected by the reviewer
	RejectIssueTmpl *tmpl.Template
)

// Setup sets up all the workflow-specific routing rules and does any other
// init necessary for workflow handling
func Setup(r *mux.Router, webPath string, c *config.Config) {
	conf = c
	basePath = webPath

	// Base path (desk view)
	var s = r.PathPrefix(basePath).Subrouter()
	s.Path("").Handler(handle(canView(homeHandler)))

	// All other paths are centered around a specific issue
	var s2 = s.PathPrefix("/{issue_id}").Subrouter()

	// Claim / unclaim handlers are for both metadata and review
	s2.Path("/claim").Methods("POST").Handler(handle(canClaim(claimIssueHandler)))
	s2.Path("/unclaim").Methods("POST").Handler(handle(ownsIssue(unclaimIssueHandler)))

	// Alias for all the middleware we call to validate issue metadata entry:
	// - User has a role which allows entering metadata
	// - User owns the issue
	// - The issue is in the right workflow step
	var canEnterMetadata = func(f HandlerFunc) http.Handler {
		return handle(canWrite(ownsIssue(issueNeedsMetadataEntry(f))))
	}

	// Issue metadata paths
	s2.Path("/metadata").Handler(canEnterMetadata(enterMetadataHandler))
	s2.Path("/metadata/save").Methods("POST").Handler(canEnterMetadata(saveMetadataHandler))
	s2.Path("/report-error").Handler(canEnterMetadata(enterErrorHandler))
	s2.Path("/report-error/save").Methods("POST").Handler(canEnterMetadata(saveErrorHandler))

	// Alias for all the middleware we call to validate issue metadata review:
	// - User has a role which allows reviewing metadata
	// - User owns the issue
	// - The issue is in the right workflow step
	var canReviewMetadata = func(f HandlerFunc) http.Handler {
		return handle(canReview(ownsIssue(issueAwaitingMetadataReview(f))))
	}

	// Review paths
	var s3 = s2.PathPrefix("/review").Subrouter()
	s3.Path("/metadata").Handler(canReviewMetadata(reviewMetadataHandler))
	s3.Path("/reject-form").Handler(canReviewMetadata(rejectIssueMetadataFormHandler))
	s3.Path("/reject").Methods("POST").Handler(canReviewMetadata(rejectIssueMetadataHandler))
	s3.Path("/approve").Methods("POST").Handler(canReviewMetadata(approveIssueMetadataHandler))

	Layout = responder.Layout.Clone()
	Layout.Path = path.Join(Layout.Path, "workflow")
	Layout.MustReadPartials("_issue_table_rows.go.html")
	DeskTmpl = Layout.MustBuild("desk.go.html")
	MetadataFormTmpl = Layout.MustBuild("metadata_form.go.html")
	ReportErrorTmpl = Layout.MustBuild("report_error.go.html")
}

// homeHandler shows claimed workflow items that need to be finished as well as
// pending items which can be claimed
func homeHandler(resp *responder.Responder, i *Issue) {
	resp.Vars.Title = "Workflow"

	// Get issues currently on user's desk
	var uid = resp.Vars.User.ID
	var issues, err = db.FindIssuesOnDesk(uid)
	if err != nil {
		logger.Errorf("Unable to find issues on user %d's desk: %s", uid, err)
		resp.Vars.Alert = fmt.Sprintf("Unable to search for issues; contact support or try again later.")
		resp.Render(responder.Empty)
		return
	}
	resp.Vars.Data["MyDeskIssues"] = wrapDBIssues(issues)

	// Get issues needing metadata
	issues, err = db.FindAvailableIssuesByWorkflowStep(db.WSReadyForMetadataEntry)
	if err != nil {
		logger.Errorf("Unable to find issues needing metadata entry: %s", err)
		resp.Vars.Alert = fmt.Sprintf("Unable to search for issues; contact support or try again later.")
		resp.Render(responder.Empty)
		return
	}
	resp.Vars.Data["PendingMetadataIssues"] = wrapDBIssues(issues)

	resp.Render(DeskTmpl)
}

// claimIssueHandler just assigns the given issue to the logged-in user and
// sets a one-week expiration
func claimIssueHandler(resp *responder.Responder, i *Issue) {
	i.WorkflowOwnerID = resp.Vars.User.ID
	i.WorkflowOwnerExpiresAt = time.Now().Add(time.Hour * 24 * 7)
	var err = i.Save()
	if err != nil {
		logger.Errorf("Unable to claim issue id %s by user %s: %s", i.ID, resp.Vars.User.Login, err)
		http.SetCookie(resp.Writer, &http.Cookie{
			Name:  "Alert",
			Value: "Unable to claim issue; contact support or try again later.",
			Path:  "/",
		})
		http.Redirect(resp.Writer, resp.Request, basePath, http.StatusFound)
		return
	}

	resp.Audit("claim", fmt.Sprintf("issue id %d", i.ID))
	http.SetCookie(resp.Writer, &http.Cookie{Name: "Info", Value: "Issue claimed successfully", Path: "/"})
	http.Redirect(resp.Writer, resp.Request, basePath, http.StatusFound)
}

// unclaimIssueHandler clears the issue's workflow data
func unclaimIssueHandler(resp *responder.Responder, i *Issue) {
	i.WorkflowOwnerID = 0
	i.WorkflowOwnerExpiresAt = time.Time{}

	var err = i.Save()
	if err != nil {
		logger.Errorf("Unable to unclaim issue id %s for user %s: %s", i.ID, resp.Vars.User.Login, err)
		http.SetCookie(resp.Writer, &http.Cookie{
			Name:  "Alert",
			Value: "Unable to unclaim issue; contact support or try again later.",
			Path:  "/",
		})
		http.Redirect(resp.Writer, resp.Request, basePath, http.StatusFound)
		return
	}

	resp.Audit("unclaim", fmt.Sprintf("issue id %d", i.ID))
	http.SetCookie(resp.Writer, &http.Cookie{Name: "Info", Value: "Issue removed from your task list", Path: "/"})
	http.Redirect(resp.Writer, resp.Request, basePath, http.StatusFound)
}

// enterMetadataHandler shows the metadata entry form for the issue
func enterMetadataHandler(resp *responder.Responder, i *Issue) {
	resp.Vars.Title = "Issue Metadata / Page Numbers"
	resp.Vars.Data["Issue"] = i
	resp.Render(MetadataFormTmpl)
}

// saveMetadataHandler takes the form data, validates it, and on success
// updates the issue in the database
func saveMetadataHandler(resp *responder.Responder, i *Issue) {
	// Set all fields and record changes for auditing / error logging
	var changes = make(map[string]string)
	var post = func(key string) string { return resp.Request.FormValue(key) }
	var save = func(key string, store *string) {
		var val = post(key)
		if val != *store {
			*store = val
			changes[key] = val
		}
	}

	save("issue_number", &i.Issue.Issue)
	save("edition_label", &i.EditionLabel)
	save("date_as_labeled", &i.DateAsLabeled)
	save("date", &i.Issue.Date)
	save("volume_number", &i.Volume)
	save("page_labels_csv", &i.PageLabelsCSV)

	var key = "edition_number"
	var val = post(key)
	var valNum, _ = strconv.Atoi(val)
	if i.Edition != valNum {
		i.Edition = valNum
		changes[key] = val
	}

	// This one's funny - we have to "deserialize" the label csv since the real
	// structure isn't what we get from the web
	i.PageLabels = strings.Split(i.PageLabelsCSV, ",")

	// Don't bother saving to the database if nothing has changed
	if len(changes) > 0 {
		// Save the issue to the database - we want to preserve the user's data even
		// if the data is invalid; invalid just means it can't be queued yet
		var err = i.Save()
		if err != nil {
			logger.Errorf("Unable to save issue id %d's metadata (POST: %#v; Changes: %#v): %s",
				i.ID, resp.Request.Form, changes, err)
			resp.Vars.Alert = "Unable to save issue; try again or contact support"
			enterMetadataHandler(resp, i)
			return
		}

		resp.Audit("save-metadata",
			fmt.Sprintf("issue id %d (POST: %#v; Changes: %#v)", i.ID, resp.Request.Form, changes))
	}

	// If the user is just saving as a draft, we don't bother validating anything
	if post("action") != "savequeue" {
		http.SetCookie(resp.Writer, &http.Cookie{Name: "Info", Value: "Saved Metadata", Path: "/"})
		http.Redirect(resp.Writer, resp.Request, i.Path("metadata"), http.StatusFound)
		return
	}

	// If there are errors, let the user know and redisplay the form
	i.ValidateMetadata()
	if len(i.Errors()) > 0 {
		var alertFormat = "Cannot queue this issue:<ul>%s</ul>"
		var errors string
		for _, err := range i.Errors() {
			errors += fmt.Sprintf("<li>%s</li>", err)
		}
		http.SetCookie(resp.Writer, &http.Cookie{Name: "Alert", Value: fmt.Sprintf(alertFormat, errors), Path: "/"})
		http.Redirect(resp.Writer, resp.Request, i.Path("metadata"), http.StatusFound)
		return
	}

	resp.Audit("queue-for-review", fmt.Sprintf("issue id %d", i.ID))
	http.SetCookie(resp.Writer, &http.Cookie{Name: "Info", Value: "Issue queued for review", Path: "/"})
	http.Redirect(resp.Writer, resp.Request, basePath, http.StatusFound)
}

// enterErrorHandler displays the form to enter an error for the given issue
func enterErrorHandler(resp *responder.Responder, i *Issue) {
	resp.Vars.Title = "Report Issue Error"
	resp.Vars.Data["Issue"] = i
	resp.Render(ReportErrorTmpl)
}

// saveErrorHandler records the error in the database, unclaims the issue, and
// flags it as needing admin attention
func saveErrorHandler(resp *responder.Responder, i *Issue) {
	i.Error = resp.Request.FormValue("error")
	if i.Error == "" {
		http.SetCookie(resp.Writer, &http.Cookie{Name: "Info", Value: "Error report empty; no action taken", Path: "/"})
		http.Redirect(resp.Writer, resp.Request, i.Path("metadata"), http.StatusFound)
		return
	}

	i.WorkflowOwnerID = 0
	i.WorkflowOwnerExpiresAt = time.Time{}
	var err = i.Save()
	if err != nil {
		logger.Errorf("Unable to save issue id %d's error (POST: %#v): %s", i.ID, resp.Request.Form, err)
		http.SetCookie(resp.Writer, &http.Cookie{
			Name:  "Alert",
			Value: "Error trying to save error report (no, the irony is not lost on us); try again or contact support",
			Path:  "/",
		})
		http.Redirect(resp.Writer, resp.Request, i.Path("report-error"), http.StatusFound)
		return
	}

	resp.Audit("report-error", fmt.Sprintf("issue id %d", i.ID))
	http.SetCookie(resp.Writer, &http.Cookie{Name: "Info", Value: "Issue error reported", Path: "/"})
	http.Redirect(resp.Writer, resp.Request, basePath, http.StatusFound)
}

func reviewMetadataHandler(resp *responder.Responder, i *Issue)          {}
func rejectIssueMetadataFormHandler(resp *responder.Responder, i *Issue) {}
func rejectIssueMetadataHandler(resp *responder.Responder, i *Issue)     {}
func approveIssueMetadataHandler(resp *responder.Responder, i *Issue)    {}
