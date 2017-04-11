package main

import (
	"log"
)

// cacheIssues calls all the individual cache functions for the
// myriad of ways we store issue information in the various locations
func cacheIssues() {
	var err error

	err = finder.FindWebBatches(opts.Siteroot, opts.CachePath)
	if err != nil {
		log.Fatalf("Error trying to cache live batched issues: %s", err)
	}
	err = finder.FindSFTPIssues(Conf.MasterPDFUploadPath)
	if err != nil {
		log.Fatalf("Error trying to cache SFTPed issues: %s", err)
	}
	err = cacheStandardIssues()
	if err != nil {
		log.Fatalf("Error trying to cache standard filesystem issues: %s", err)
	}
	err = finder.FindDiskBatches(Conf.BatchOutputPath)
	if err != nil {
		log.Fatalf("Error trying to cache batches: %s", err)
	}
}

// cacheStandardIssues deals with all the various locations for issues which
// are not in a batch directory structure.  This doesn't mean they haven't been
// batched, just that the directory uses the somewhat consistent pdf-to-chronam
// structure `topdir/sftpnameOrLCCN/yyyy-mm-dd/`
func cacheStandardIssues() error {
	var locs = []string{
		Conf.MasterPDFBackupPath,
		Conf.PDFPageReviewPath,
		Conf.PDFPagesAwaitingMetadataReview,
		Conf.PDFIssuesAwaitingDerivatives,
		Conf.ScansAwaitingDerivatives,
		Conf.PDFPageBackupPath,
		Conf.PDFPageSourcePath,
	}

	for _, loc := range locs {
		var err = finder.FindStandardIssues(loc)
		if err != nil {
			return err
		}
	}

	return nil
}
