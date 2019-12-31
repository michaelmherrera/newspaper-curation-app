package jobs

import (
	"os"
	"path/filepath"

	"github.com/uoregon-libraries/gopkg/fileutil"
)

// moveIssue is used by various issue move jobs to consistently validate and
// move the source issue directory into the workflow location
func moveIssue(ij *IssueJob, path string) bool {
	var iKey = ij.Issue.Key()

	// Verify new path will work
	var oldLocation = ij.DBIssue.Location
	var newLocation = filepath.Join(path, ij.DBIssue.HumanName)
	if !fileutil.MustNotExist(newLocation) {
		ij.Logger.Errorf("Destination %q already exists for issue %q", newLocation, iKey)
		return false
	}

	// Move the issue directory to the workflow path
	var wipLocation = filepath.Join(path, ".wip-"+ij.DBIssue.HumanName)
	ij.Logger.Infof("Copying %q to %q", oldLocation, wipLocation)
	var err = fileutil.CopyDirectory(oldLocation, wipLocation)
	if err != nil {
		ij.Logger.Errorf("Unable to copy issue %q directory: %s", iKey, err)
		return false
	}
	err = os.RemoveAll(oldLocation)
	if err != nil {
		ij.Logger.Errorf("Unable to clean up issue %q after copying to WIP directory: %s", iKey, err)
		return false
	}
	err = os.Rename(wipLocation, newLocation)
	if err != nil {
		ij.Logger.Errorf("Unable to rename WIP issue directory (%q -> %q) post-copy: %s", wipLocation, newLocation, err)
		return false
	}
	ij.Issue.Location = newLocation

	// Make sure non-NCA apps can read the new location
	os.Chmod(newLocation, 0755)

	// The issue has been moved, so a failure updating the record isn't a failure
	// and can only be logged loudly
	ij.DBIssue.Location = ij.Issue.Location
	err = ij.DBIssue.Save()
	if err != nil {
		ij.Logger.Criticalf("Unable to update Issue's location for id %d: %s", ij.DBIssue.ID, err)
	}

	return true
}
