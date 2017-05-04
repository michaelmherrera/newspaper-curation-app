package sftphandler

import (
	"path"
)

// TitlePath returns the relative path to the given title's issue list page
func TitlePath(name string) string {
	return path.Join(basePath, name)
}

// IssuePath returns the relative path to the given issue's PDF list page
func IssuePath(title, issue string) string {
	return path.Join(basePath, title, issue)
}

// PDFPath returns the relative path to view a given PDF file
func PDFPath(title, issue, filename string) string {
	return path.Join(basePath, title, issue, filename)
}

