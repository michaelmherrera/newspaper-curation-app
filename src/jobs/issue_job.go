package jobs

import (
	"fmt"

	"github.com/uoregon-libraries/gopkg/logger"
	"github.com/uoregon-libraries/newspaper-curation-app/src/db"
	"github.com/uoregon-libraries/newspaper-curation-app/src/schema"
)

// IssueJob wraps the Job type to add things needed in all jobs tied to
// specific issues
type IssueJob struct {
	*Job
	Issue            *schema.Issue
	DBIssue          *db.Issue
	updateWorkflowCB func()
}

// NewIssueJob setups up an IssueJob from a database Job, centralizing the
// common validations and data manipulation
func NewIssueJob(dbJob *db.Job) *IssueJob {
	var dbi, err = db.FindIssue(dbJob.ObjectID)
	if err != nil {
		logger.Criticalf("Unable to find issue for job %d: %s", dbJob.ID, err)
		return nil
	}

	var si *schema.Issue
	si, err = dbi.SchemaIssue()
	if err != nil {
		logger.Criticalf("Unable to prepare a schema.Issue for database issue %d: %s", dbi.ID, err)
		return nil
	}

	return &IssueJob{
		Job:     NewJob(dbJob),
		DBIssue: dbi,
		Issue:   si,
	}
}

// Subdir returns a subpath to the job issue's directory for consistent
// directory naming and single-level paths
func (ij *IssueJob) Subdir() string {
	if ij.DBIssue.HumanName == "" {
		ij.DBIssue.HumanName = fmt.Sprintf("%s-%s-%d",
			ij.Issue.Title.LCCN, ij.Issue.DateEdition(), ij.DBIssue.ID)
	}
	return ij.DBIssue.HumanName
}

// WIPDir returns a hidden name for a work-in-progress directory to allow
// processing / copying to occur in a way that won't mess up end users
func (ij *IssueJob) WIPDir() string {
	return ".wip-" + ij.Subdir()
}

// UpdateWorkflow sets the attached issue's WorkflowStep if the job has defined
// "ExtraData".  The optional updateWorkflowCB is called if defined, and
// then the issue job is saved.  At this point, however, the job is complete,
// so all we can do is loudly log failures.
func (ij *IssueJob) UpdateWorkflow() {
	var ws = schema.WorkflowStep(ij.db.ExtraData)
	if ws != schema.WSNil {
		ij.DBIssue.WorkflowStep = ws
	}
	if ij.updateWorkflowCB != nil {
		ij.updateWorkflowCB()
	}

	var err = ij.DBIssue.Save()
	if err != nil {
		ij.Logger.Criticalf("Unable to update issue (dbid %d) workflow post-job: %s", ij.DBIssue.ID, err)
	}
}

// ObjectLocation implements the Processor interface, returning the issue's
// current location
func (ij *IssueJob) ObjectLocation() string {
	return ij.DBIssue.Location
}
