// Package responder contains all the general functionality necessary for
// responding to a given server request: template setup, user auth checks,
// rendering of pages to an http.ResponseWriter
package responder

import (
	"db"
	"encoding/base64"
	"html/template"

	"net/http"
	"time"
	"user"
	"version"
	"web/tmpl"
	"web/webutil"

	"github.com/uoregon-libraries/gopkg/logger"
)

// GenericVars holds anything specialized that doesn't make sense to have in PageVars
type GenericVars map[string]interface{}

// PageVars is the generic list of data all pages may need, and the catch-all
// "Data" map for specialized one-off data
type PageVars struct {
	Title   string
	Version string
	Webroot string
	Alert   template.HTML
	Info    template.HTML
	User    *user.User
	Data    GenericVars
}

// Responder wraps common response logic
type Responder struct {
	Writer  http.ResponseWriter
	Request *http.Request
	Vars    *PageVars
}

// Response generates a Responder with basic data all pages will need: request,
// response writer, and user
func Response(w http.ResponseWriter, req *http.Request) *Responder {
	var u = user.FindByLogin(GetUserLogin(w, req))
	u.IP = GetUserIP(req)
	return &Responder{Writer: w, Request: req, Vars: &PageVars{User: u, Data: make(GenericVars)}}
}

// injectDefaultTemplateVars sets up default variables used in multiple templates
func (r *Responder) injectDefaultTemplateVars() {
	r.Vars.Webroot = webutil.Webroot
	r.Vars.Version = version.Version
	if r.Vars.Title == "" {
		r.Vars.Title = "ODNP Admin"
	}
}

// Render uses the responder's data to render the given template
func (r *Responder) Render(t *tmpl.Template) {
	r.injectDefaultTemplateVars()
	var cookie, err = r.Request.Cookie("Alert")
	if err == nil && cookie.Value != "" {
		r.Vars.Alert = template.HTML(cookie.Value)
		// TODO: This is such a horrible hack.  We need real session data management.
		if len(r.Vars.Alert) > 6 && r.Vars.Alert[0:6] == "base64" {
			var data, err = base64.StdEncoding.DecodeString(string(r.Vars.Alert[6:]))
			r.Vars.Alert = template.HTML(string(data))
			if err != nil {
				r.Vars.Alert = ""
			}
		}
		http.SetCookie(r.Writer, &http.Cookie{Name: "Alert", Value: "", Expires: time.Time{}, Path: "/"})
	}
	cookie, err = r.Request.Cookie("Info")
	if err == nil && cookie.Value != "" {
		r.Vars.Info = template.HTML(cookie.Value)
		http.SetCookie(r.Writer, &http.Cookie{Name: "Info", Value: "", Expires: time.Time{}, Path: "/"})
	}

	err = t.Execute(r.Writer, r.Vars)
	if err != nil {
		logger.Errorf("Unable to render template %#v: %s", t.Name, err)
	}
}

// Audit stores an audit log in the database and logs to the command line if
// the database audit fails
func (r *Responder) Audit(action, msg string) {
	var u = r.Vars.User
	var err = db.CreateAuditLog(u.IP, u.Login, action, msg)
	if err != nil {
		logger.Criticalf("Unable to write AuditLog{%s (%s), %q, %s}: %s", u.Login, u.IP, action, msg, err)
	}
}

// Error sets up the Alert var and sends the appropriate header to the browser.
// If msg is empty, the status text from the http package is used.
func (r *Responder) Error(status int, msg string) {
	r.Writer.WriteHeader(status)
	if msg == "" {
		msg = http.StatusText(status)
	}
	r.Vars.Alert = template.HTML(msg)
	r.Render(Empty)
}
