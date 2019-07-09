package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/uoregon-libraries/gopkg/tmpl"
)

// Error is just a string that returns itself when its Error method is
// called so that const strings can implement the error interface
type Error string

func (e Error) Error() string {
	return string(e)
}

// errInvalidFormUID is used when a user is trying to load a form that
// doesn't exist (it may have expired or the server restarted or something)
const errInvalidFormUID = Error("invalid form uid")

// errUnownedForm occurs when a user has a form uid associated with a
// different user
const errUnownedForm = Error("user doesn't own requested form uid")

// Templates are global here because we need them accessible from multiple functions
var metadata *tmpl.Template
var upload *tmpl.Template

// getFormNonAJAX gets the form data from getUploadForm.  On any errors (not
// validation of data, but form errors), we automatically redirect the client
// and return ok==false so the caller knows to just exit, not handle anything
// further.
func (r *responder) getFormNonAJAX() (f *uploadForm, ok bool) {
	var err error
	f, err = r.getUploadForm()
	if err == nil {
		return f, true
	}

	switch err {
	case errInvalidFormUID:
		r.sess.SetAlertFlash("Unable to find session data - your form may have timed out")
		r.redirectSubpath("upload", http.StatusSeeOther)

	case errUnownedForm:
		r.redirectSubpath("upload", http.StatusSeeOther)

	default:
		r.server.logger.Errorf("Unknown error parsing form data: %#v", err)
		r.sess.SetAlertFlash("Unknown error parsing your form data.  Please reload and try again.")
		r.redirectSubpath("upload", http.StatusSeeOther)
	}

	return f, false
}

// getUploadForm retrieves the user's form from their session and populates it
// with their request data.  On backend or validation problems, an error is
// returned.
func (r *responder) getUploadForm() (*uploadForm, error) {
	var user = r.sess.GetString("user")

	// Retrieve form or register a new one
	var uid = r.req.FormValue("uid")

	// If the form is new, there can be no errors
	if uid == "" {
		return registerForm(user), nil
	}

	var f = findForm(uid)
	if f == nil {
		r.server.logger.Warnf("Session user %q trying to claim invalid form uid %q", user, uid)
		return nil, errInvalidFormUID
	}

	// Validate ownership
	if f != nil && f.Owner != user {
		r.server.logger.Errorf("Session user %q trying to claim form owned by %q", user, f.Owner)
		return nil, errUnownedForm
	}

	return f, nil
}

func (s *srv) uploadFormHandler() http.Handler {
	metadata = s.layout.MustBuild("upload-metadata.go.html")
	upload = s.layout.MustBuild("upload-files.go.html")

	return s.route(func(r *responder) {
		var form, ok = r.getFormNonAJAX()
		if !ok {
			return
		}

		var next = r.req.FormValue("nextstep")
		var data = map[string]interface{}{"Form": form}

		switch next {
		// If we're on the metadata step, we don't try to parse incoming fields or
		// validate the form
		case "", "metadata":
			r.render(metadata, data)

		case "upload":
			var err = form.parseMetadata(r.req)
			switch err {
			case nil:
				r.render(upload, data)
			case errInvalidDate:
				data["Alert"] = "Invalid date: make sure you use YYYY-MM-DD format"
				r.render(metadata, data)
			default:
				s.logger.Errorf("Unhandled form parse error: %s", err)
				data["Alert"] = "Invalid metadata"
				r.render(metadata, data)
			}

		default:
			s.logger.Warnf("Invalid next step: %q", next)
		}
	})
}

func (s *srv) uploadAJAXReceiver() http.Handler {
	return s.route(func(r *responder) {
		var uid = r.req.FormValue("uid")
		if uid == "" {
			// This can actually happen on a canceled upload, so we respond, but don't log anything
			r.ajaxError("upload error: no form", http.StatusBadRequest)
			return
		}

		var form, err = r.getUploadForm()
		if err != nil {
			s.logger.Errorf("Error processing form for AJAX request: %s", err)
			r.ajaxError("upload error: invalid form", http.StatusBadRequest)
			return
		}

		var uerr = r.getAJAXUpload(form)
		if uerr != nil {
			if uerr.error != nil {
				s.logger.Errorf("Error reading file upload for AJAX request: %s", uerr.error)
			}
			if uerr.message == "" {
				uerr.message = "unable to process your upload - please try again"
			}
			if uerr.code == 0 {
				uerr.code = http.StatusInternalServerError
			}
			r.ajaxError(uerr.message, uerr.code)
			return
		}

		r.w.Write([]byte("ok"))
	})
}

// uploadError gives us an error value that can tell us how to report the error
// in logs separately from how we want to present it to the user
type uploadError struct {
	error
	code    int
	message string
}

// getAJAXUpload pulls the AJAX upload and stores it into a temporary file.
// The file is stored in the form's Files list or else an error is returned.
func (r *responder) getAJAXUpload(form *uploadForm) *uploadError {
	var file, header, err = r.req.FormFile("myfile")
	if err != nil {
		return &uploadError{error: err}
	}

	var out *os.File
	out, err = ioutil.TempFile(os.TempDir(), form.UID+"-")
	if err != nil {
		return &uploadError{error: fmt.Errorf("unable to create temp file for file upload: %s", err)}
	}

	// Compute and check checksum while writing the file
	var h = sha256.New()
	var reader = io.TeeReader(file, h)

	var n int64
	n, err = io.Copy(out, reader)
	if n != header.Size {
		os.Remove(out.Name())
		return &uploadError{error: fmt.Errorf("only wrote partial file")}
	}
	if err != nil {
		os.Remove(out.Name())
		return &uploadError{error: fmt.Errorf("unable to write to tempfile")}
	}

	err = out.Close()
	if err != nil {
		os.Remove(out.Name())
		return &uploadError{error: fmt.Errorf("unable to close tempfile")}
	}

	var checksum = h.Sum(nil)
	for _, file := range form.Files {
		if bytes.Equal(file.sum, checksum) {
			os.Remove(out.Name())
			return &uploadError{
				error:   nil,
				code:    http.StatusBadRequest,
				message: "Skipping: this is a duplicate of " + file.Name,
			}
		}
	}

	var f = &uploadedFile{
		path: out.Name(),
		Name: header.Filename,
		Size: header.Size,
		sum:  checksum,
	}

	form.Files = append(form.Files, f)
	return nil
}
