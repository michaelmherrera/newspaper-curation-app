package jobs

import (
	"config"
	"fileutil"
	"os"
	"path/filepath"
)

// SFTPIssueMover is a job that gets queued up when an SFTP issue is considered
// ready for processing.  It moves the issue to the workflow area and, upon
// success, queues up a page split job.
type SFTPIssueMover struct {
	*IssueJob
}

// Process moves the SFTP issue directory to the workflow area
func (im *SFTPIssueMover) Process(config *config.Config) {
	var iKey = im.Issue.Key()

	// Verify new path will work
	var newLocation = filepath.Join(config.WorkflowPath, iKey)
	if !fileutil.MustNotExist(newLocation) {
		im.Logger.Error("Destination %q already exists for issue %q", newLocation, iKey)
		return
	}

	// Move the issue directory to the workflow path
	var wipLocation = newLocation + "-wip"
	os.MkdirAll(filepath.Dir(wipLocation), 0700)
	im.Logger.Info("Queueing %q to %q", im.Issue.Location, wipLocation)
	var err = fileutil.CopyDirectory(im.Issue.Location, wipLocation)
	if err != nil {
		im.Logger.Error("Unable to copy issue %q directory: %s", iKey, err)
		return
	}
	err = os.RemoveAll(im.Issue.Location)
	if err != nil {
		im.Logger.Error("Unable to clean up issue %q after copying to WIP directory: %s", iKey, err)
		return
	}
	err = os.Rename(wipLocation, newLocation)
	if err != nil {
		im.Logger.Error("Unable to rename WIP issue directory (%q -> %q) post-copy: %s", wipLocation, newLocation, err)
		return
	}
	im.Issue.Location = newLocation

	im.Status = string(JobStatusSuccessful)
	err = im.Save()
	if err != nil {
		im.Logger.Critical("Unable to update workflow metadata after moving sftp issue %q: %s", iKey, err)
		return
	}
}
