package jobs

import (
	"path/filepath"

	"github.com/uoregon-libraries/newspaper-curation-app/src/config"
	"github.com/uoregon-libraries/newspaper-curation-app/src/db"
	"github.com/uoregon-libraries/newspaper-curation-app/src/schema"
)

// These constants let us define arg names in a way that ensures we don't screw
// up by setting an arg and then misspelling the reader of said arg
const (
	wsArg   = "WorkflowStep"
	bsArg   = "BatchStatus"
	locArg  = "Location"
	srcArg  = "Source"
	destArg = "Destination"
)

// PrepareJobAdvanced gets a job of any kind set up with sensible defaults
func PrepareJobAdvanced(t db.JobType, args map[string]string) *db.Job {
	return db.NewJob(t, args)
}

// PrepareIssueJobAdvanced is a way to get an issue job ready with the
// necessary base values, but not save it immediately, to allow for more
// advanced job semantics: specifying that the job shouldn't run immediately,
// should queue a specific job ID after completion, should set the WorkflowStep
// to a custom value rather than whatever the job would normally do, etc.
func PrepareIssueJobAdvanced(t db.JobType, issue *db.Issue, args map[string]string) *db.Job {
	var j = PrepareJobAdvanced(t, args)
	j.ObjectID = issue.ID
	j.ObjectType = db.JobObjectTypeIssue
	return j
}

// PrepareBatchJobAdvanced gets a batch job ready for being used elsewhere
func PrepareBatchJobAdvanced(t db.JobType, batch *db.Batch, args map[string]string) *db.Job {
	var j = PrepareJobAdvanced(t, args)
	j.ObjectID = batch.ID
	j.ObjectType = db.JobObjectTypeBatch
	return j
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
			j.Status = string(db.JobStatusOnHold)
		}
		var err = j.SaveOp(op)
		if err != nil {
			return err
		}
		lastJobID = j.ID
	}

	return op.Err()
}

func makeWSArgs(ws schema.WorkflowStep) map[string]string {
	return map[string]string{wsArg: string(ws)}
}

func makeBSArgs(bs string) map[string]string {
	return map[string]string{bsArg: string(bs)}
}

func makeLocArgs(loc string) map[string]string {
	return map[string]string{locArg: loc}
}

func makeRenameArgs(src, dest string) map[string]string {
	return map[string]string{
		srcArg:  src,
		destArg: dest,
	}
}

// QueueSFTPIssueMove queues up an issue move into the workflow area followed
// by a page-split and then a move to the page review area
func QueueSFTPIssueMove(issue *db.Issue) error {
	return QueueSerial(
		PrepareIssueJobAdvanced(db.JobTypeSetIssueWS, issue, makeWSArgs(schema.WSAwaitingProcessing)),
		PrepareIssueJobAdvanced(db.JobTypeMoveIssueToWorkflow, issue, nil),
		PrepareIssueJobAdvanced(db.JobTypePageSplit, issue, nil),
		PrepareIssueJobAdvanced(db.JobTypeMoveIssueToPageReview, issue, nil),
		PrepareIssueJobAdvanced(db.JobTypeSetIssueWS, issue, makeWSArgs(schema.WSAwaitingPageReview)),
	)
}

// QueueMoveIssueForDerivatives creates jobs to move issues into the workflow
// and then immediately generate derivatives
func QueueMoveIssueForDerivatives(issue *db.Issue) error {
	return QueueSerial(
		PrepareIssueJobAdvanced(db.JobTypeSetIssueWS, issue, makeWSArgs(schema.WSAwaitingProcessing)),
		PrepareIssueJobAdvanced(db.JobTypeMoveIssueToWorkflow, issue, nil),
		PrepareIssueJobAdvanced(db.JobTypeMakeDerivatives, issue, nil),
		PrepareIssueJobAdvanced(db.JobTypeSetIssueWS, issue, makeWSArgs(schema.WSReadyForMetadataEntry)),
	)
}

// QueueFinalizeIssue creates and queues jobs that get an issue ready for
// batching.  Currently this means generating the METS XML file and copying
// master PDFs (if born-digital) into the issue directory.
func QueueFinalizeIssue(issue *db.Issue) error {
	return QueueSerial(
		PrepareIssueJobAdvanced(db.JobTypeBuildMETS, issue, nil),
		PrepareIssueJobAdvanced(db.JobTypeArchiveMasterFiles, issue, nil),
		PrepareJobAdvanced(db.JobTypeKillDir, makeLocArgs(issue.MasterBackupLocation)),
		PrepareIssueJobAdvanced(db.JobTypeSetIssueMasterLoc, issue, makeLocArgs("")),
		PrepareIssueJobAdvanced(db.JobTypeSetIssueWS, issue, makeWSArgs(schema.WSReadyForBatching)),
	)
}

// QueueMakeBatch sets up the jobs for generating a batch on disk: generating
// the directories and hard-links, making the batch XML, putting the batch
// where it can be loaded onto staging, and generating the bagit manifest.
// Nothing can happen automatically after all this until the batch is verified
// on staging.
func QueueMakeBatch(batch *db.Batch, c *config.Config) error {
	var root = c.BatchOutputPath
	var wipDir = filepath.Join(root, ".wip-"+batch.FullName())
	var finalDir = filepath.Join(root, batch.FullName())
	return QueueSerial(
		PrepareBatchJobAdvanced(db.JobTypeCreateBatchStructure, batch, makeLocArgs(wipDir)),
		PrepareBatchJobAdvanced(db.JobTypeSetBatchLocation, batch, makeLocArgs(wipDir)),
		PrepareBatchJobAdvanced(db.JobTypeMakeBatchXML, batch, nil),
		PrepareJobAdvanced(db.JobTypeRenameDir, makeRenameArgs(wipDir, finalDir)),
		PrepareBatchJobAdvanced(db.JobTypeSetBatchLocation, batch, makeLocArgs(finalDir)),
		PrepareBatchJobAdvanced(db.JobTypeSetBatchStatus, batch, makeBSArgs(db.BatchStatusQCReady)),
		PrepareBatchJobAdvanced(db.JobTypeWriteBagitManifest, batch, nil),
	)
}
