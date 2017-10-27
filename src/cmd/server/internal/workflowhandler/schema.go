package workflowhandler

import (
	"db"
	"fmt"
	"html/template"
	"logger"
	"path"
	"schema"
	"strconv"
	"time"
)

// Issue wraps the DB issue, and decorates them with display-friendly functions
type Issue struct {
	*db.Issue
	si *schema.Issue
}

func wrapDBIssue(dbIssue *db.Issue) *Issue {
	var si, err = dbIssue.SchemaIssue()

	// This shouldn't realistically happen, so we log and return nothing
	if err != nil {
		logger.Errorf("Unable to get schema.Issue for issue id %d: %s", dbIssue.ID, err)
		return nil
	}

	return &Issue{Issue: dbIssue, si: si}
}

func wrapDBIssues(dbIssues []*db.Issue) []*Issue {
	var list []*Issue
	for _, dbIssue := range dbIssues {
		var i = wrapDBIssue(dbIssue)
		if i == nil {
			return nil
		}
		list = append(list, i)
	}

	return list
}

// Name returns the issue's title's name
func (i *Issue) Title() string {
	return i.si.Title.Name
}

// LCCN returns the issue's title's LCCN
func (i *Issue) LCCN() string {
	return i.si.Title.LCCN
}

// Date returns the human-friendly date string
func (i *Issue) Date() string {
	return i.si.DateStringReadable()
}

// TaskDescription returns a human-friendly explanation of the current place
// this issue is within the workflow
func (i *Issue) TaskDescription() string {
	switch i.WorkflowStep {
	case db.WSAwaitingPageReview:
		return "Ready for page review (renaming files / validating raw PDFs / TIFFs)"

	case db.WSReadyForMetadataEntry:
		return "Awaiting metadata entry / page numbering"

	case db.WSAwaitingMetadataReview:
		return "Awaiting review (metadata and page numbers)"

	default:
		logger.Criticalf("Invalid workflow step for issue %d: %q", i.ID, i.WorkflowStepString)
		return "UNKNOWN!"
	}
}

// WorkflowExpiration returns the date and time of "workflow expiration": when
// this item is no longer claimed by the workflow owner
func (i *Issue) WorkflowExpiration() string {
	return i.WorkflowOwnerExpiresAt.Format("2006-01-02 at 15:04")
}

// actionButton creates an action button wrapped by a one-off form for actions
// related to a single issue
func (i *Issue) actionButton(label, actionPath, classes string) template.HTML {
	return template.HTML(fmt.Sprintf(
		`<form action="%s" method="POST" class="actions"><button type="submit" class="btn %s">%s</button></form>`,
		i.Path(actionPath), classes, label))
}

// actionLink creates a link to the given action; for non-destructive actions
// like visiting a form page
func (i *Issue) actionLink(label, actionPath, classes string) template.HTML {
	return template.HTML(fmt.Sprintf(`<a href="%s" class="%s">%s</a>`, i.Path(actionPath), classes, label))
}

// IsOwned returns true if the owner ID is nonzero *and* the workflow owner
// expiration time has not passed
func (i *Issue) IsOwned() bool {
	return i.WorkflowOwnerID != 0 && time.Now().Before(i.WorkflowOwnerExpiresAt)
}

// Actions returns the action link HTML for each possible action the owner can
// take for this issue
func (i *Issue) Actions() []template.HTML {
	var actions []template.HTML

	if i.IsOwned() {
		switch i.WorkflowStep {
		case db.WSReadyForMetadataEntry:
			actions = append(actions, i.actionLink("Metadata", "metadata", ""))
			actions = append(actions, i.actionLink("Page Numbering", "page-numbering", ""))

		case db.WSAwaitingMetadataReview:
			actions = append(actions, i.actionLink("Metadata", "review/metadata", ""))
			actions = append(actions, i.actionLink("Page Numbering", "review/page-numbering", ""))
		}

		actions = append(actions, i.actionButton("Unclaim", "/unclaim", "btn-danger"))
	} else {
		actions = append(actions, i.actionButton("Claim", "/claim", "btn-primary"))
	}

	return actions
}

// Path returns the path for any basic actions on this issue
func (i *Issue) Path(actionPath string) string {
	return path.Join(basePath, strconv.Itoa(i.ID), actionPath)
}
