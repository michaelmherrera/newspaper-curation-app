package titlehandler

import (
	"db"
	"regexp"
	"schema"
	"sort"
	"strings"
)

var re = regexp.MustCompile(`[^A-Za-z0-9]+`)
var noWordRE = regexp.MustCompile(`\W+`)

// Title wraps a db.Title for web display
type Title struct {
	*db.Title
	SortName string
}

// WrapTitle converts a db.Title to a Title, giving it a useful "SortName"
// based on its name (stripped of common prefixes) and LCCN
func WrapTitle(t *db.Title) *Title {
	return &Title{t, strings.ToLower(re.ReplaceAllString(schema.TrimCommonPrefixes(t.Name)+t.LCCN, "-"))}
}

// WrapTitles takes a db.TitleList and wraps each title individually
func WrapTitles(list db.TitleList) []*Title {
	var titles = make([]*Title, len(list))
	for i, t := range list {
		titles[i] = WrapTitle(t)
	}

	return titles
}

// SortTitles does an in-place sort on the given title list, relying solely on
// the SortName string
func SortTitles(list []*Title) {
	sort.Slice(list, func(i, j int) bool { return list[i].SortName < list[j].SortName })
}

// TitlesDiffer returns true if the MARC title isn't the same as the name we've
// given the title.  We strip all non-word characters for the comparison.
func (t *Title) TitlesDiffer() bool {
	var mt = noWordRE.ReplaceAllString(t.MARCTitle+t.MARCLocation, "")
	var n = noWordRE.ReplaceAllString(t.Name, "")
	return mt != n
}
