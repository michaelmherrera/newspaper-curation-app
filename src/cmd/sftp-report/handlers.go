package main

import (
	"fmt"
	"log"
	"net/http"
	"presenter"
	"sftp"
	"time"
	"user"

	"github.com/gorilla/mux"
)

func getUserLogin(w http.ResponseWriter, req *http.Request) string {
	var l string
	if DEBUG {
		l = req.URL.Query().Get("debuguser")
		if l == "" {
			var cookie, err = req.Cookie("debuguser")
			if err == nil {
				l = cookie.Value
			}
		}
		if l == "nil" {
			l = ""
			http.SetCookie(w, &http.Cookie{Name: "debuguser", Value: "", Expires: time.Time{}})
		} else {
			http.SetCookie(w, &http.Cookie{Name: "debuguser", Value: l})
		}
	}

	if l == "" {
		l = req.Header.Get("X-Remote-User")
	}

	return l
}

// Response generates a Responder with basic data all pages will need: request,
// response writer, and user
func Response(w http.ResponseWriter, req *http.Request) *Responder {
	var u = user.FindByLogin(getUserLogin(w, req))
	return &Responder{Writer: w, Request: req, Vars: &PageVars{User: u, Data: make(GenericVars)}}
}

// Middleware function to send back no-cache header
func nocache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=0, must-revalidate")
		next.ServeHTTP(w, r)
	})
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var u = getUserLogin(w, r)
		if u != "" {
			log.Printf("Request: [%s] %s", u, r.URL)
		} else {
			log.Printf("Request: [nil] %s", r.URL)
		}
		next.ServeHTTP(w, r)
	})
}

// mustHavePrivilege denies access to pages if there's no logged-in user, or
// there is a user but the user isn't allowed to perform a particular action
func mustHavePrivilege(priv *user.Privilege, f http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var u = user.FindByLogin(getUserLogin(w, r))
		var roles []*user.Role
		if u != nil {
			roles = u.Roles()
		}

		if priv.AllowedByAny(roles) {
			f(w, r)
			return
		}

		var resp = Response(w, r)
		resp.Vars.Title = "Insufficient Privileges"
		w.WriteHeader(http.StatusForbidden)
		resp.Render("insufficient-privileges")
	})
}

// LoadPublishers takes a responder and attempts to load the publisher list
// into it.  If the list can't be loaded, an HTTP error is sent out and the
// return is false.
func LoadPublishers(r *Responder) bool {
	var pubList, err = sftp.BuildPublishers(SFTPPath)
	if err != nil {
		log.Printf("ERROR: Couldn't load publishers in %s: %s", SFTPPath, err)
		http.Error(r.Writer, "Unable to load publisher list!", 500)
		return false
	}

	r.Vars.Publishers = presenter.PublisherList(pubList)
	return true
}

// findPublisher attempts to load the publisher list, then find and return the
// publisher specified in the URL If no publisher is found (or loading
// publishers fails), nil is returned, and the caller should do nothing, as
// http headers / rendering is already done.
func findPublisher(r *Responder) *presenter.Publisher {
	if !LoadPublishers(r) {
		return nil
	}

	var pubName = mux.Vars(r.Request)["publisher"]
	var publisher *presenter.Publisher
	for _, p := range r.Vars.Publishers {
		if p.Name == pubName {
			publisher = p
		}
	}

	if publisher == nil {
		r.Vars.Alert = fmt.Sprintf("Unable to find publisher %#v", pubName)
		r.Render("empty")
		return nil
	}

	return publisher
}

// findIssue attempts to find the publisher specified in the URL and then the
// issue for that publisher, also specified in the URL.  If found, the issue is
// returned.  If not found, some kind of contextual error will be displayed to
// the end user and the caller should do nothing.
func findIssue(r *Responder) *presenter.Issue {
	var publisher = findPublisher(r)
	if publisher == nil {
		return nil
	}

	var name = mux.Vars(r.Request)["issue"]
	var issue *presenter.Issue
	for _, iss := range publisher.Issues {
		if iss.Name == name {
			issue = iss
		}
	}

	if issue == nil {
		r.Vars.Alert = fmt.Sprintf("Unable to find issue %#v for publisher %#v", name, publisher.Name)
		r.Render("empty")
		return nil
	}

	return issue
}

// HomeHandler spits out the publisher list
func HomeHandler(w http.ResponseWriter, req *http.Request) {
	var r = Response(w, req)
	if !LoadPublishers(r) {
		return
	}

	r.Vars.Title = "SFTP Publisher List"
	r.Render("home")
}

// PublisherHandler prints a list of issues for a given publisher
func PublisherHandler(w http.ResponseWriter, req *http.Request) {
	var r = Response(w, req)
	var publisher = findPublisher(r)
	if publisher == nil {
		return
	}

	r.Vars.Data["Publisher"] = publisher
	r.Vars.Title = "SFTP Issues for " + publisher.Name
	r.Render("publisher")
}

// IssueHandler prints a list of pages for a given issue
func IssueHandler(w http.ResponseWriter, req *http.Request) {
	var r = Response(w, req)
	var issue = findIssue(r)
	if issue == nil {
		return
	}

	r.Vars.Data["Issue"] = issue
	r.Vars.Title = fmt.Sprintf("SFTP PDFs for %s, issue %s", issue.Publisher.Name, issue.Name)
	r.Render("issue")
}
