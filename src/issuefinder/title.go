package issuefinder

import (
	"chronam"
	"db"
	"fmt"
	"httpcache"
	"schema"
)

// findFilesystemTitle looks up the title by its given path and returns it or
// creates a new one if its "titleName" is in the database.  "titleName" can
// be LCCN or SFTP directory depending on the type of directory.
func (f *Finder) findFilesystemTitle(titleName, path string) *schema.Title {
	if f.titleByLoc[path] == nil {
		var t = createDBTitle(titleName)
		t.Location = path
		f.addTitle(t)
	}
	return f.titleByLoc[path]
}

// createDBTitle looks up the title in the the database by directory name and LCCN
func createDBTitle(titleName string) *schema.Title {
	var dbTitle = db.FindTitleByDirectory(titleName)
	if dbTitle == nil {
		dbTitle = db.FindTitleByLCCN(titleName)
	}
	if dbTitle == nil {
		return nil
	}

	return &schema.Title{
		LCCN:               dbTitle.LCCN,
		Name:               dbTitle.MarcTitle,
		PlaceOfPublication: dbTitle.MarcLocation,
	}
}

// findOrCreateUnknownFilesystemTitle looks up the title by path and returns it
// or creates a new one.  This should only be used for titles for which we have
// no metadata: when LCCN is the only data available, the title is incomplete.
func (f *Finder) findOrCreateUnknownFilesystemTitle(lccn, path string) *schema.Title {
	if f.titleByLoc[path] == nil {
		f.addTitle(&schema.Title{LCCN: lccn, Location: path})
	}
	return f.titleByLoc[path]
}

// addTitle pushes the title into the global titles list and caches it by its
// location field
func (f *Finder) addTitle(title *schema.Title) {
	f.Titles = append(f.Titles, title)
	f.titleByLoc[title.Location] = title
}

// findOrCreateWebTitle looks up the title by its given URI and returns it or
// requests the URI to create, cache, and return a new one
func (f *Finder) findOrCreateWebTitle(c *httpcache.Client, uri string) (*schema.Title, error) {
	if f.titleByLoc[uri] != nil {
		return f.titleByLoc[uri], nil
	}

	var request = httpcache.AutoRequest(uri, "titles")
	var contents, err = c.GetCachedBytes(request)
	if err != nil {
		return nil, fmt.Errorf("unable to GET %#v: %s", uri, err)
	}
	var tJSON *chronam.TitleJSON
	tJSON, err = chronam.ParseTitleJSON(contents)
	if err != nil {
		return nil, fmt.Errorf("unable to parse title JSON for %#v: %s", uri, err)
	}

	f.addTitle(&schema.Title{
		LCCN:               tJSON.LCCN,
		Name:               tJSON.Name,
		PlaceOfPublication: tJSON.PlaceOfPublication,
		Location:           uri,
	})
	return f.titleByLoc[uri], nil
}
