// Package schema houses simple data types for titles, issues, batches, etc.
// Types which live here are generally meant to be very general-case rather
// than trying to hold all possible information for all possible use cases.
//
// Except... a Location field exists on all structures because the workflow
// allows for multiple occurrences of metadata for any of the schema items.
// They could be on the filesystem or the web.  And in the case of errors,
// which we need to be able to detect, there can be dupes that need to be
// reported and figured out.
package schema

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/uoregon-libraries/gopkg/fileutil"
)

// WorkflowStep describes the location within the workflow any issue can exist
// - this is basically a more comprehensive list than what's in the database in
// order to capture every possible location: live batches, sftped issues
// awaiting processing, etc.
type WorkflowStep string

// All possible statuses an issue could have
const (
	// WSNil should only be used to indicate a workflow step is irrelevant or else unset
	WSNil                    WorkflowStep = ""
	WSSFTP                                = "SFTPUpload"
	WSScan                                = "ScanUpload"
	WSAwaitingProcessing                  = "AwaitingProcessing"
	WSAwaitingPageReview                  = "AwaitingPageReview"
	WSReadyForMetadataEntry               = "ReadyForMetadataEntry"
	WSAwaitingMetadataReview              = "AwaitingMetadataReview"
	WSReadyForMETSXML                     = "ReadyForMETSXML"
	WSReadyForBatching                    = "ReadyForBatching"
	WSInProduction                        = "InProduction"
)

// Batch represents high-level batch information
type Batch struct {
	// MARCOrgCode tells us the organization responsible for the images in the batch
	MARCOrgCode string

	// A batch's keyword is normally short, such as "horsetail", but our in-house
	// batches have much longer keywords to ensure uniqueness
	Keyword string

	// Usually 1, but I've seen "_ver02" batches occasionally
	Version int

	// Issues links the issues which are part of this batch
	Issues IssueList

	// Location is where this batch can be found, either a URL or filesystem path
	Location string
}

// ParseBatchname creates a Batch by splitting up the full name string
func ParseBatchname(fullname string) (*Batch, error) {
	// All batches must have the format "batch_MARCORGCODE_NAME_ver##"
	var parts = strings.Split(fullname, "_")

	// This is really obnoxious, but we can only test for too few parse.  Despite
	// the spec's claim that the batch keyword must not have underscores, some
	// live batches do.  I'm lookin' at you, "courage_3".
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid batch format")
	}

	if parts[0] != "batch" {
		return nil, fmt.Errorf(`batches must begin with "batch_"`)
	}

	var l = len(parts)
	var b = &Batch{}
	var ver string
	parts, ver = parts[:l-1], parts[l-1]
	b.MARCOrgCode, b.Keyword = parts[1], strings.Join(parts[2:], "_")

	if len(ver) != 5 || ver[:3] != "ver" {
		return nil, fmt.Errorf("invalid version format")
	}

	b.Version, _ = strconv.Atoi(ver[3:])
	if b.Version < 1 {
		return nil, fmt.Errorf("invalid version value")
	}

	return b, nil
}

// Fullname is the full batch name
func (b *Batch) Fullname() string {
	var parts = []string{"batch", b.MARCOrgCode, b.Keyword, fmt.Sprintf("ver%02d", b.Version)}
	return strings.Join(parts, "_")
}

// TSV returns a string uniquely identifying this batch by location as well
// as name, and an issue count to offer some verification or reporting
func (b *Batch) TSV() string {
	return fmt.Sprintf("%s\t%s\t%06d", b.Location, b.Fullname(), len(b.Issues))
}

// AddIssue adds the issue to this batch's list, and sets the issue's batch
func (b *Batch) AddIssue(i *Issue) {
	b.Issues = append(b.Issues, i)
	i.Batch = b
}

// Title is a publisher's information, unique per LCCN
type Title struct {
	LCCN               string
	Name               string
	PlaceOfPublication string

	// Issues contains the list of issues associated with a single title; though
	// this can be derived by iterating over all the issues, it's useful to store
	// them here, too
	Issues IssueList

	// Location is where the title was found on disk or web; not actual Title metadata
	Location string
}

// TSV returns a string representing this title uniquely by including its
// location and a count of issues.  The issue count won't help us deserialize,
// but the purpose is just for data verification and simple reporting.
func (t *Title) TSV() string {
	return fmt.Sprintf("%s\t%s\t%s\t%s\t%06d", t.Location, t.LCCN, t.Name, t.PlaceOfPublication, len(t.Issues))
}

// AddIssue adds the issue to this title's list, and sets the issue's title
func (t *Title) AddIssue(i *Issue) *Issue {
	t.Issues = append(t.Issues, i)
	i.Title = t
	return i
}

// GenericTitle returns a title with the same generic information, but none of
// the data which is tied to a specific title on the filesystem or website:
// location and issue list
func (t *Title) GenericTitle() *Title {
	return &Title{LCCN: t.LCCN, Name: t.Name, PlaceOfPublication: t.PlaceOfPublication}
}

// TitleList is a simple slice of titles for easier built-in sorting and
// identifying a unique list of all titles
type TitleList []*Title

// trimCommonPrefixes strips "The", "A", and "An" from the string if they're at
// the beginning, and removes leading spaces
func trimCommonPrefixes(s string) string {
	s = strings.TrimPrefix(s, "The")
	s = strings.TrimPrefix(s, "the")
	s = strings.TrimPrefix(s, "A")
	s = strings.TrimPrefix(s, "a")
	s = strings.TrimPrefix(s, "An")
	s = strings.TrimPrefix(s, "an")
	return strings.TrimSpace(s)
}

// SortByName sorts the titles by their name, using location and lccn when
// names are the same
func (list TitleList) SortByName() {
	sort.Slice(list, func(i, j int) bool {
		var a, b = strings.ToLower(trimCommonPrefixes(list[i].Name)), strings.ToLower(trimCommonPrefixes(list[j].Name))

		if a == b {
			a, b = list[i].Location, list[j].Location
		}
		if a == b {
			a, b = list[i].LCCN, list[j].LCCN
		}

		return a < b
	})
}

// Unique returns a new list containing generic versions of each unique LCCN
func (list TitleList) Unique() TitleList {
	var l2 TitleList
	var seen = make(map[string]bool)
	for _, title := range list {
		if seen[title.LCCN] {
			continue
		}

		seen[title.LCCN] = true
		l2 = append(l2, title.GenericTitle())
	}
	return l2
}

// Issue is an extremely basic encapsulation of an issue's high-level data
type Issue struct {
	MARCOrgCode string
	Title       *Title
	RawDate     string // This is the date as seen on the filesystem when the issue was uploaded
	Edition     int
	Batch       *Batch
	Files       []*File

	// Location is where this issue can be found, either a URL or filesystem path
	Location string

	WorkflowStep WorkflowStep
}

// condensedDate returns the date in a consistent format for use in issue key TSV output
func (i *Issue) condensedDate() string {
	return strings.Replace(i.RawDate, "-", "", -1)
}

// DateEdition returns the combination of condensed date (no hyphens) and
// two-digit edition number for use in issue keys and other places we need the
// "local" unique string
func (i *Issue) DateEdition() string {
	return fmt.Sprintf("%s%02d", i.condensedDate(), i.Edition)
}

// Key returns the unique string that represents this issue
func (i *Issue) Key() string {
	return fmt.Sprintf("%s/%s", i.Title.LCCN, i.DateEdition())
}

// TSV gives us something which can be used to uniquely identify all aspects of
// this issue's data for reporting and/or data verification
func (i *Issue) TSV() string {
	var bString = "nil"
	if i.Batch != nil {
		bString = strings.Replace(i.Batch.TSV(), "\t", "\\t", -1)
	}
	var tString = strings.Replace(i.Title.TSV(), "\t", "\\t", -1)
	var fileNames []string
	for _, file := range i.Files {
		fileNames = append(fileNames, file.Name)
	}
	return fmt.Sprintf("%s\t%s\t%s\t%s%02d\t%s\t%s", bString, tString, i.Location, i.condensedDate(),
		i.Edition, i.WorkflowStep, strings.Join(fileNames, ","))
}

// FindFiles clears the issue's file list and then reads everything in the
// issue directory, appending it to the now-empty list.  This will silently
// fail when the issue's location is invalid, not readable, or isn't an
// absolute path beginning with "/".  This is only meant for issues already
// discovered on the filesystem.
func (i *Issue) FindFiles() {
	i.Files = nil

	if i.Location[0] != '/' {
		return
	}

	var infos, _ = fileutil.ReaddirSorted(i.Location)
	for _, file := range fileutil.InfosToFiles(infos) {
		var loc = filepath.Join(i.Location, file.Name)
		i.Files = append(i.Files, &File{File: file, Issue: i, Location: loc})
	}
}

// IsLive returns true if the issue both has a batch *and* the batch appears to
// be on the live site
func (i *Issue) IsLive() bool {
	return i.Batch != nil && i.Batch.Location[0:4] == "http"
}

// WorkflowIdentification returns a human-readable explanation of where an
// issue lives currently is in the workflow - currently used for adding to
// "likely duplicate of ..."
func (i *Issue) WorkflowIdentification() string {
	switch i.WorkflowStep {
	case WSSFTP:
		return "a born-digital issue waiting for processing"

	case WSScan:
		return "a scanned issue waiting for processing"

	case WSAwaitingProcessing:
		return "a pending issue"

	case WSAwaitingPageReview:
		return "an issue awaiting page reordering / renumbering"

	case WSReadyForMetadataEntry:
		return "an issue awaiting metadata entry"

	case WSAwaitingMetadataReview:
		return "an issue awaiting metadata review"

	case WSReadyForBatching:
		return "an issue waiting to be batched"

	case WSInProduction:
		return "a live issue in batch " + i.Batch.Fullname()

	default:
		return fmt.Sprintf("an unknown issue (location: %q)", i.Location)
	}
}

// IssueList groups a bunch of issues together
type IssueList []*Issue

// SortByKey modifies the IssueList in place so they're sorted alphabetically
// by issue key.  In cases where the keys are the same, the TSV is used to
// ensure sorting is still consistent, if not ideal.
func (list IssueList) SortByKey() {
	sort.Slice(list, func(i, j int) bool {
		var kA, kB = list[i].Key(), list[j].Key()
		if kA != kB {
			return kA < kB
		}

		return list[i].TSV() < list[j].TSV()
	})
}

// File just gives fileutil.File a location and issue pointer
type File struct {
	*fileutil.File
	Location string
	Issue    *Issue
}
