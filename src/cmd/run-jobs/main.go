// This script finds and runs pending jobs, scans for page review issues which
// have been renamed and are ready for derivatives, and will eventually perform
// all automated processes Batch Maker has to offer.

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	flags "github.com/jessevdk/go-flags"
	"github.com/uoregon-libraries/gopkg/interrupts"
	"github.com/uoregon-libraries/gopkg/wordutils"
	"github.com/uoregon-libraries/newspaper-curation-app/src/config"
	"github.com/uoregon-libraries/newspaper-curation-app/src/db"
	"github.com/uoregon-libraries/newspaper-curation-app/src/internal/logger"
	"github.com/uoregon-libraries/newspaper-curation-app/src/jobs"
	"github.com/uoregon-libraries/newspaper-curation-app/src/schema"
)

var runners struct {
	sync.Mutex
	list []*jobs.Runner
}

var isDone int32

func addRunner(r *jobs.Runner) {
	runners.Lock()
	runners.list = append(runners.list, r)
	runners.Unlock()
}

func quit() {
	atomic.StoreInt32(&isDone, 1)
	runners.Lock()
	for _, r := range runners.list {
		r.Stop()
	}
	runners.Unlock()
}

func done() bool {
	return atomic.LoadInt32(&isDone) == 1
}

// Command-line options
var opts struct {
	ConfigFile string `short:"c" long:"config" description:"path to NCA config file" required:"true"`
}

var p *flags.Parser
var titles = make(map[string]*schema.Title)

var validQueues = make(map[string]bool)
var validQueueList []string

// wrap is a helper to wrap a usage message at 80 characters and print a
// newline afterward
func wrap(msg string) {
	fmt.Fprint(os.Stderr, wordutils.Wrap(msg, 80))
	fmt.Fprintln(os.Stderr)
}

func wrapBullet(msg string) {
	var lines = strings.Split(wordutils.Wrap(msg, 80), "\n")
	for i, line := range lines {
		if i > 0 {
			line = "  " + line
		}
		fmt.Fprint(os.Stderr, line+"\n")
	}
}

func usageFail(format string, args ...interface{}) {
	wrap(fmt.Sprintf(format, args...))
	fmt.Fprintln(os.Stderr)
	p.WriteHelp(os.Stderr)

	fmt.Fprintln(os.Stderr)
	wrap("Valid actions:")
	fmt.Fprintln(os.Stderr)
	wrapBullet("* requeue <job id> [<job id>...]: Creates new jobs by cloning and " +
		`closing the given failed jobs.  Only jobs with a status of "failed" can be requeued.`)
	wrapBullet("* watchall: Runs watchers for all queues and the page review " +
		"issues in a relatively sane configuration.  Use this unless you need the " +
		`more complex granularity offered by "watch" and "watch-page-review"`)
	wrapBullet("* watch <queue name> [<queue name>...]: Watches for jobs in the " +
		"given queue(s), processing them in a loop until CTRL+C is pressed")
	wrapBullet("* watch-page-review: Watches for issues awaiting page review " +
		"(reordering or other manual processing) which are ready to be moved for " +
		"metadata entry.  No job is associated with this action, hence it must run on " +
		"its own, and should only have one copy running at a time.")
	wrapBullet("* force-rerun <job id> [<job id>...]: Creates new jobs by cloning the " +
		"given jobs and running the new clones.  Extra metadata is removed to avoid " +
		"as many side-effects as possible.  This is NOT a good idea unless you know " +
		"exactly what the job(s) you're cloning can affect.")

	fmt.Fprintln(os.Stderr)
	wrap(fmt.Sprintf("Valid queue names: %s", strings.Join(validQueueList, ", ")))

	os.Exit(1)
}

func getOpts() (*config.Config, []string) {
	p = flags.NewParser(&opts, flags.HelpFlag|flags.PassDoubleDash)
	var args, err = p.Parse()

	if err != nil {
		usageFail("Error: %s", err)
	}

	var c *config.Config
	c, err = config.Parse(opts.ConfigFile)
	if err != nil {
		logger.Fatalf("Invalid configuration: %s", err)
	}

	err = db.Connect(c.DatabaseConnect)
	if err != nil {
		logger.Fatalf("Unable to connect to the database: %s", err)
	}

	return c, args
}

// setupValidQueueNames copies in the list of valid queues for easier validation
func setupValidQueueNames() {
	for _, jType := range db.ValidJobTypes {
		var jt = string(jType)
		validQueues[jt] = true
		validQueueList = append(validQueueList, jt)
	}
}

func main() {
	setupValidQueueNames()
	var c, args = getOpts()
	if len(args) < 1 {
		usageFail("Error: you must specify an action")
	}

	// On CTRL-C / kill, try to finish the current task before exiting
	interrupts.TrapIntTerm(quit)

	var action string
	action, args = args[0], args[1:]
	switch action {
	case "requeue":
		requeue(args)
	case "watch":
		watch(c, args...)
	case "watch-page-review":
		watchPageReview(c)
	case "watchall":
		runAllQueues(c)
	default:
		usageFail("Error: invalid action")
	}
}

func requeue(ids []string) {
	if len(ids) == 0 {
		usageFail("Error: the requeue action requires at least one job id")
	}

	for _, idString := range ids {
		retryJob(idString)
	}
}

func findJob(idString string) *jobs.Job {
	var id, _ = strconv.Atoi(idString)
	if id == 0 {
		logger.Errorf("Invalid job id %q", idString)
		return nil
	}

	var j = jobs.Find(id)
	if j == nil {
		logger.Errorf("No job found with id %d", id)
		return nil
	}

	return j
}

func retryJob(idString string) {
	var j = findJob(idString)
	if j == nil {
		return
	}

	var failStatus = db.JobStatusFailed
	var dj = j.DBJob()
	if dj.Status != string(failStatus) {
		logger.Errorf("Cannot requeue job id %d: status is %s (it must be %s to requeue)", dj.ID, dj.Status, failStatus)
		return
	}

	logger.Infof("Requeuing job %d", dj.ID)
	var err = j.Requeue()
	if err != nil {
		logger.Errorf("Unable to requeue job %d: %s", dj.ID, err)
	}
}

func validateJobQueue(queue string) {
	if !validQueues[queue] {
		usageFail("Invalid job queue %q", queue)
	}
}

func watch(c *config.Config, queues ...string) {
	if len(queues) == 0 {
		usageFail("Error: you must specify one or more queues to watch")
	}

	var jobTypes = make([]db.JobType, len(queues))
	for i, queue := range queues {
		validateJobQueue(queue)
		jobTypes[i] = db.JobType(queue)
	}
	watchJobTypes(c, jobTypes...)
}

func watchJobTypes(c *config.Config, jobTypes ...db.JobType) {
	var r = jobs.NewRunner(c, jobTypes...)
	addRunner(r)
	r.Watch(time.Second * 10)
}

func watchPageReview(c *config.Config) {
	logger.Infof("Watching page review folders")

	var nextAttempt time.Time
	for !done() {
		if time.Now().After(nextAttempt) {
			scanPageReviewIssues(c)
			nextAttempt = time.Now().Add(10 * time.Minute)
		}

		// Try not to eat all the CPU
		time.Sleep(time.Second)
	}
}

// runAllQueues fires up multiple goroutines to watch all the queues in a
// fairly sane way so that important processes like moving SFTP issues can
// happen quickly, while CPU-bound processes won't fight each other.
func runAllQueues(c *config.Config) {
	waitFor(
		func() { watchPageReview(c) },
		func() {
			// Jobs which are exclusively disk IO are in the first runner to avoid
			// too much FS stuff hapenning concurrently
			watchJobTypes(c,
				db.JobTypeArchiveMasterFiles,
				db.JobTypeSyncDir,
				db.JobTypeKillDir,
				db.JobTypeWriteBagitManifest,
			)
		},
		func() {
			// Jobs which primarily use CPU are grouped next, so we aren't trying to
			// share CPU too much
			watchJobTypes(c,
				db.JobTypePageSplit,
				db.JobTypeMakeDerivatives,
			)
		},
		func() {
			// Fast - but not instant - jobs are here: file renaming, hard-linking,
			// running templates for very simple XML output, etc.  These typically
			// take very little CPU or disk IO, but they aren't "critical" jobs that
			// need to be real-time.
			watchJobTypes(c,
				db.JobTypeBuildMETS,
				db.JobTypeCreateBatchStructure,
				db.JobTypeMakeBatchXML,
				db.JobTypeRenameDir,
				db.JobTypeCleanFiles,
			)
		},
		func() {
			// Extremely fast data-setting jobs get a custom runner that operates
			// every second to ensure nearly real-time updates to things like a job's
			// workflow state
			var r = jobs.NewRunner(c,
				db.JobTypeSetIssueWS,
				db.JobTypeSetIssueMasterLoc,
				db.JobTypeSetIssueLocation,
				db.JobTypeSetBatchStatus,
				db.JobTypeSetBatchLocation,
			)
			addRunner(r)
			r.Watch(time.Second * 1)
		},
	)
}

// waitFor runs all the passed-in functions concurrently and returns when
// they're all complete
func waitFor(fns ...func()) {
	var wg sync.WaitGroup

	for _, fn1 := range fns {
		wg.Add(1)
		go func(fn2 func()) {
			fn2()
			wg.Done()
		}(fn1)
	}

	wg.Wait()
}
