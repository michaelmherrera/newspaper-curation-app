package jobs

import (
	"time"

	"github.com/uoregon-libraries/newspaper-curation-app/src/db"
	"github.com/uoregon-libraries/newspaper-curation-app/src/schema"
)

// PrepareJobAdvanced gets a job of any kind set up with sensible defaults
func PrepareJobAdvanced(t JobType) *db.Job {
	return &db.Job{
		Type:   string(t),
		Status: string(JobStatusPending),
		RunAt:  time.Now(),
	}
}

// PrepareIssueJobAdvanced is a way to get an issue job ready with the
// necessary base values, but not save it immediately, to allow for more
// advanced job semantics: specifying that the job shouldn't run immediately,
// should queue a specific job ID after completion, should set the WorkflowStep
// to a custom value rather than whatever the job would normally do, etc.
func PrepareIssueJobAdvanced(t JobType, issue *db.Issue, nextWS schema.WorkflowStep) *db.Job {
	var j = PrepareJobAdvanced(t)
	j.ObjectID = issue.ID
	j.ObjectType = db.JobObjectTypeIssue
	j.ExtraData = string(nextWS)
	return j
}

// PrepareBatchJobAdvanced gets a batch job ready for being used elsewhere
func PrepareBatchJobAdvanced(t JobType, batch *db.Batch) *db.Job {
	var j = PrepareJobAdvanced(t)
	j.ObjectID = batch.ID
	j.ObjectType = db.JobObjectTypeBatch
	return j
}

func queueIssueJob(t JobType, issue *db.Issue, nextWS schema.WorkflowStep) error {
	return PrepareIssueJobAdvanced(t, issue, nextWS).Save()
}

// QueueSerial attempts to save the jobs (in a transaction), setting the first
// one as ready to run while the others become effectively dependent on the
// prior job in the list
func QueueSerial(jobs ...*db.Job) error {
	var op = db.DB.Operation()
	op.BeginTransaction()
	defer op.EndTransaction()

	// Iterate over jobs in reverse so we can set the prior job's next-run id
	// without saving things twice
	var lastJobID int
	for i := len(jobs) - 1; i >= 0; i-- {
		var j = jobs[i]
		j.QueueJobID = lastJobID
		if i != 0 {
			j.Status = string(JobStatusOnHold)
		}
		var err = j.SaveOp(op)
		if err != nil {
			return err
		}
		lastJobID = j.ID
	}

	return op.Err()
}

// QueueSFTPIssueMove queues up an issue move into the workflow area followed
// by a page-split and then a move to the page review area
func QueueSFTPIssueMove(issue *db.Issue) error {
	return QueueSerial(
		PrepareIssueJobAdvanced(JobTypeMoveIssueToWorkflow, issue, schema.WSNil),
		PrepareIssueJobAdvanced(JobTypePageSplit, issue, schema.WSNil),
		PrepareIssueJobAdvanced(JobTypeMoveIssueToPageReview, issue, schema.WSAwaitingPageReview),
	)
}

// QueueMoveIssueForDerivatives creates jobs to move issues into the workflow
// and then immediately generate derivatives
func QueueMoveIssueForDerivatives(issue *db.Issue) error {
	return QueueSerial(
		PrepareIssueJobAdvanced(JobTypeMoveIssueToWorkflow, issue, schema.WSNil),
		PrepareIssueJobAdvanced(JobTypeMakeDerivatives, issue, schema.WSReadyForMetadataEntry),
	)
}

// QueueFinalizeIssue creates and queues jobs that get an issue ready for
// batching.  Currently this means generating the METS XML file and copying
// master PDFs (if born-digital) into the issue directory.
func QueueFinalizeIssue(issue *db.Issue) error {
	return QueueSerial(
		PrepareIssueJobAdvanced(JobTypeBuildMETS, issue, schema.WSNil),
		PrepareIssueJobAdvanced(JobTypeMoveMasterFiles, issue, schema.WSReadyForBatching),
	)
}

// QueueMakeBatch sets up the jobs for generating a batch on disk: generating
// the directories and hard-links, making the batch XML, putting the batch
// where it can be loaded onto staging, and generating the bagit manifest.
// Nothing can happen automatically after all this until the batch is verified
// on staging.
func QueueMakeBatch(batch *db.Batch) error {
	// Ensure the batch is flagged properly after it's ready
	var moveJob = PrepareBatchJobAdvanced(JobTypeMoveBatchToReadyLocation, batch)
	moveJob.ExtraData = string(db.BatchStatusQCReady)

	return QueueSerial(
		PrepareBatchJobAdvanced(JobTypeCreateBatchStructure, batch),
		PrepareBatchJobAdvanced(JobTypeMakeBatchXML, batch),
		moveJob, PrepareBatchJobAdvanced(JobTypeWriteBagitManifest, batch),
	)
}
