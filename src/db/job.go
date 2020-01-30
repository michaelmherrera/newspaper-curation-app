package db

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Nerdmaster/magicsql"
)

// Object types for consistently inserting into the database
const (
	JobObjectTypeBatch = "batch"
	JobObjectTypeIssue = "issue"
)

// JobType represents all possible jobs the system queues and processes
type JobType string

// The full list of job types
const (
	JobTypeSetIssueWS            JobType = "set_issue_workflow_step"
	JobTypeSetIssueMasterLoc     JobType = "set_issue_master_backup_location"
	JobTypeSetBatchStatus        JobType = "set_batch_status"
	JobTypePageSplit             JobType = "page_split"
	JobTypeMoveIssueToWorkflow   JobType = "move_issue_to_workflow"
	JobTypeMoveIssueToPageReview JobType = "move_issue_to_page_review"
	JobTypeMakeDerivatives       JobType = "make_derivatives"
	JobTypeBuildMETS             JobType = "build_mets"
	JobTypeArchiveMasterFiles    JobType = "archive_master_files"
	JobTypeSetBatchLocation      JobType = "set_batch_location"
	JobTypeCreateBatchStructure  JobType = "create_batch_structure"
	JobTypeMakeBatchXML          JobType = "make_batch_xml"
	JobTypeWriteBagitManifest    JobType = "write_bagit_manifest"
	JobTypeSyncDir               JobType = "sync_directory"
	JobTypeKillDir               JobType = "delete_directory"
	JobTypeRenameDir             JobType = "rename_directory"
)

// ValidJobTypes is the full list of job types which can exist in the jobs
// table, for use in validating command-line job queue processing
var ValidJobTypes = []JobType{
	JobTypeSetIssueWS,
	JobTypeSetIssueMasterLoc,
	JobTypeSetBatchStatus,
	JobTypePageSplit,
	JobTypeMoveIssueToWorkflow,
	JobTypeMoveIssueToPageReview,
	JobTypeMakeDerivatives,
	JobTypeBuildMETS,
	JobTypeArchiveMasterFiles,
	JobTypeSetBatchLocation,
	JobTypeCreateBatchStructure,
	JobTypeMakeBatchXML,
	JobTypeWriteBagitManifest,
	JobTypeSyncDir,
	JobTypeKillDir,
	JobTypeRenameDir,
}

// JobStatus represents the different states in which a job can exist
type JobStatus string

// The full list of job statuses
const (
	JobStatusOnHold     JobStatus = "on_hold"     // Jobs waiting for another job to complete
	JobStatusPending    JobStatus = "pending"     // Jobs needing to be processed
	JobStatusInProcess  JobStatus = "in_process"  // Jobs which have been taken by a worker but aren't done
	JobStatusSuccessful JobStatus = "success"     // Jobs which were successful
	JobStatusFailed     JobStatus = "failed"      // Jobs which are complete, but did not succeed
	JobStatusFailedDone JobStatus = "failed_done" // Jobs we ignore - e.g., failed jobs which were rerun
)

// JobLog is a single log entry attached to a job
type JobLog struct {
	ID        int `sql:",primary"`
	JobID     int
	CreatedAt time.Time `sql:",readonly"`
	LogLevel  string
	Message   string
}

// A Job is anything the app needs to process and track in the background
type Job struct {
	ID          int       `sql:",primary"`
	CreatedAt   time.Time `sql:",readonly"`
	StartedAt   time.Time `sql:",noinsert"`
	CompletedAt time.Time `sql:",noinsert"`
	Type        string    `sql:"job_type"`
	ObjectID    int
	ObjectType  string
	Status      string
	logs        []*JobLog

	// The job won't be run until sometime after RunAt; usually it's very close,
	// but the daemon doesn't pound the database every 5 milliseconds, so it can
	// take a little bit
	RunAt time.Time

	// XDat holds extra information, encoded as JSON, any job might need - e.g.,
	// the issue's next workflow step if the job is successful.  This shouldn't
	// be modified directly; use Args instead (which is why we've chosen such an
	// odd name for this field).
	XDat string `sql:"extra_data"`

	// Args contains the decoded values from XDat
	Args map[string]string `sql:"-"`

	// QueueJobID tells us which job (if any) should be queued up after this one
	// completes successfully
	QueueJobID int
}

// NewJob sets up a job of the given type as a pending job that's ready to run
// right away
func NewJob(t JobType, args map[string]string) *Job {
	if args == nil {
		args = make(map[string]string)
	}
	return &Job{
		Type:   string(t),
		Status: string(JobStatusPending),
		RunAt:  time.Now(),
		Args:   args,
	}
}

// FindJob gets a job by its id
func FindJob(id int) (*Job, error) {
	var jobs, err = findJobs("id = ?", id)
	if len(jobs) == 0 {
		return nil, err
	}
	return jobs[0], err
}

// findJobs wraps all the job finding functionality so helpers can be
// one-liners.  This is purposely *not* exported to enforce a stricter API.
//
// NOTE: All instantiations from the database must go through this function to
// properly set up their args map!
func findJobs(where string, args ...interface{}) ([]*Job, error) {
	var op = DB.Operation()
	op.Dbg = Debug
	var list []*Job
	op.Select("jobs", &Job{}).Where(where, args...).AllObjects(&list)
	for _, j := range list {
		var err = j.decodeXDat()
		if err != nil {
			return nil, fmt.Errorf("error decoding job %d: %s", j.ID, err)
		}
	}
	return list, op.Err()
}

// PopNextPendingJob is a helper for locking the database to pull the oldest
// job with one of the given types and set it to in-process
func PopNextPendingJob(types []JobType) (*Job, error) {
	var op = DB.Operation()
	op.Dbg = Debug

	op.BeginTransaction()
	defer op.EndTransaction()

	// Wrangle the IN pain...
	var j = &Job{}
	var args []interface{}
	var placeholders []string
	args = append(args, string(JobStatusPending), time.Now())
	for _, t := range types {
		args = append(args, string(t))
		placeholders = append(placeholders, "?")
	}

	var clause = fmt.Sprintf("status = ? AND run_at <= ? AND job_type IN (%s)", strings.Join(placeholders, ","))
	if !op.Select("jobs", &Job{}).Where(clause, args...).Order("created_at").First(j) {
		return nil, op.Err()
	}

	j.decodeXDat()
	j.Status = string(JobStatusInProcess)
	j.StartedAt = time.Now()
	j.SaveOp(op)

	return j, op.Err()
}

// FindJobsByStatus returns all jobs that have the given status
func FindJobsByStatus(st JobStatus) ([]*Job, error) {
	return findJobs("status = ?", string(st))
}

// FindJobsByStatusAndType returns all jobs of the given status and type
func FindJobsByStatusAndType(st JobStatus, t JobType) ([]*Job, error) {
	return findJobs("status = ? AND job_type = ?", string(st), string(t))
}

// FindRecentJobsByType grabs all jobs of the given type which were created
// within the given duration or are still pending, for use in pulling lists of
// issues which are in the process of doing something
func FindRecentJobsByType(t JobType, d time.Duration) ([]*Job, error) {
	var pendingJobs, otherJobs []*Job
	var err error

	pendingJobs, err = FindJobsByStatusAndType(JobStatusPending, t)
	if err != nil {
		return nil, err
	}
	otherJobs, err = findJobs("status <> ? AND job_type = ? AND created_at > ?",
		string(JobStatusPending), string(t), time.Now().Add(-d))
	if err != nil {
		return nil, err
	}

	return append(pendingJobs, otherJobs...), nil
}

// FindJobsForIssueID returns all jobs tied to the given issue
func FindJobsForIssueID(id int) ([]*Job, error) {
	return findJobs("object_id = ?", id)
}

// Logs lazy-loads all logs for this job from the database
func (j *Job) Logs() []*JobLog {
	if j.logs == nil {
		var op = DB.Operation()
		op.Dbg = Debug
		op.Select("job_logs", &JobLog{}).Where("job_id = ?", j.ID).AllObjects(&j.logs)
	}

	return j.logs
}

// WriteLog stores a log message on this job
func (j *Job) WriteLog(level string, message string) error {
	var l = &JobLog{JobID: j.ID, LogLevel: level, Message: message}
	var op = DB.Operation()
	op.Dbg = Debug
	op.Save("job_logs", l)
	return op.Err()
}

// decodeXDat attempts to parse XDat
func (j *Job) decodeXDat() error {
	// Special case 1: no extra data means we don't try to decode it
	if j.XDat == "" {
		return nil
	}

	// Special case 2: raw extra data - we hard-code whatever is in XDat as
	// being a "legacy" value so the app at least doesn't crash, and we could
	// convert the data if necessary.
	if j.XDat[0:3] != "v.2" {
		j.Args = make(map[string]string)
		j.Args["legacy"] = j.XDat
		return nil
	}

	return json.Unmarshal([]byte(j.XDat[3:]), &j.Args)
}

// encodeArgs turns our args map into JSON.  We ignore errors here because it's
// not actually possible for Go's built-in JSON encoder to fail when we're just
// encoding a map of string->string.
func (j *Job) encodeArgs() {
	if len(j.Args) == 0 {
		j.XDat = ""
		return
	}
	var b, _ = json.Marshal(j.Args)
	j.XDat = "v.2" + string(b)
}

// Save creates or updates the Job in the jobs table
func (j *Job) Save() error {
	var op = DB.Operation()
	op.Dbg = Debug
	return j.SaveOp(op)
}

// SaveOp creates or updates the job in the jobs table using a custom operation
func (j *Job) SaveOp(op *magicsql.Operation) error {
	j.encodeArgs()
	op.Save("jobs", j)
	return op.Err()
}
