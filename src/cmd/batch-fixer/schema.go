package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/uoregon-libraries/gopkg/fileutil"
	"github.com/uoregon-libraries/newspaper-curation-app/src/db"
	"github.com/uoregon-libraries/newspaper-curation-app/src/jobs"
)

// Batch extends a db.Batch with functionality for fixing, re-queueing, etc.
type Batch struct {
	db     *db.Batch
	Issues IssueList
}

// FindBatch looks up a batch in the database, then pulls all its issues
func FindBatch(id int) (*Batch, error) {
	var batch, err = db.FindBatch(id)
	if err != nil {
		return nil, fmt.Errorf("database error: %s", err.Error())
	}

	if batch == nil {
		return nil, fmt.Errorf("id not in database")
	}

	var b = &Batch{db: batch}
	err = b.loadIssues()
	if err != nil {
		return nil, fmt.Errorf("error loading batch issues: %s", err)
	}

	return b, nil
}

// Fail deletes all batch files from disk - these are all bagit files or
// hard-links, so we can easily replace everything removed.  The batch location
// is cleared, and its status is then set to "failed_qc" so it's clear it needs
// to be reprocessed in some way.
func (b *Batch) Fail() error {
	if !fileutil.IsDir(b.db.Location) {
		return fmt.Errorf("removing batch files: %q does not exist", b.db.Location)
	}

	var err = os.RemoveAll(b.db.Location)
	if err != nil {
		return fmt.Errorf("removing batch files: %s", err)
	}

	b.db.Status = db.BatchStatusFailedQC
	b.db.Location = ""
	err = b.db.Save()
	if err != nil {
		return fmt.Errorf("updating database status: %s", err)
	}

	return nil
}

// Requeue puts the batch back into the processor queue for being rebuilt.
// This doesn't do anything to the associated issue list: requeuing will simply
// result in a new batch on disk with the issues currently assigned to it.
func (b *Batch) Requeue() error {
	if b.db.Status != db.BatchStatusPending {
		return fmt.Errorf("status must be %s", db.BatchStatusPending)
	}
	if b.db.Location != "" {
		return fmt.Errorf("batch location field must be empty")
	}
	return jobs.QueueMakeBatch(b.db)
}

func (b *Batch) loadIssues() error {
	var dbIssues, err = b.db.Issues()

	b.Issues = make(IssueList, len(dbIssues))
	for i, dbi := range dbIssues {
		b.Issues[i] = &Issue{db: dbi}
	}
	b.Issues.SortByKey()
	return err
}

// Issue extends a db.Issue with functionality for pulling the issue off a
// batch, rejecting it (which, post-batch, means more than just rejection when
// it's in the metadata review phase), etc.
type Issue struct {
	db *db.Issue
}

// RemoveMETS attempts to remove the METS XML file, returning an error if any
// problems occur *except* the file already being gone, since that may be a
// sign this was called previously, or somebody had to handle it manually.  We
// verify sanity first by checking that the issue's directory does indeed
// exist (and is a directory as opposed to a file).
func (i *Issue) RemoveMETS() error {
	var si, err = i.db.SchemaIssue()
	if err != nil {
		return fmt.Errorf("unable to get a schema.Issue from the db.Issue: %s", err)
	}

	// Make sure the dir exists, since lack of a mets file isn't a failure
	if !fileutil.IsDir(i.db.Location) {
		return fmt.Errorf("issue directory %q does not exist; aborting", i.db.Location)
	}

	err = os.Remove(si.METSFile())
	if !os.IsNotExist(err) && err != nil {
		return fmt.Errorf("unable to remove METS file: %s", err)
	}

	return nil
}

// IssueList is a simple wrapper around a slice of issues to add functionality
// for easier sorting
type IssueList []*Issue

// SortByKey modifies the IssueList in place so they're sorted alphabetically
// by issue key
func (list IssueList) SortByKey() {
	sort.Slice(list, func(i, j int) bool {
		var kA, kB = list[i].db.Key(), list[j].db.Key()
		if kA != kB {
			return kA < kB
		}

		return list[i].db.ID < list[j].db.ID
	})
}
