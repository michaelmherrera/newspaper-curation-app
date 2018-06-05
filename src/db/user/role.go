package user

import (
	"regexp"
	"strings"
)

// A Role defines a grouping of privileges
type Role struct {
	Name string
	Desc string
}

var oneLineRegexp = regexp.MustCompile(`\s*\n\s*`)

// oneline turns a multi-line string into a single line by collapsing newlines
// surrounded by any amount of whitespace (space, tab, etc.) into a single
// ASCII space.
func oneline(s string) string {
	return oneLineRegexp.ReplaceAllString(s, " ")
}

// Hard-coded list of roles
var (
	RoleAny   = newRole("-any-", "N/A")
	RoleAdmin = newRole("admin",
		`No restrictions.  These users can modify data not meant for modification
		outside of initial setup and data repair situations, such as sftp
		user/password, LCCNs which have already been validated, etc.`)
	RoleTitleManager = newRole("title manager",
		`Has access to add and change newspaper titles, including the ability to
		view the sftp authorization information`)
	RoleIssueCurator = newRole("issue curator",
		`Can modify issue metadata and push issues to the review queue`)
	RoleIssueReviewer = newRole("issue reviewer",
		`Can modify issue metadata, push issues to the review queue, and mark issues as reviewed`)
	RoleUserManager = newRole("user manager",
		`Can add, edit, and delete users.  User managers can assign any rights to
		others which have been assigned to them.`)
	RoleMOCManager      = newRole("marc org code manager", "Has access to add new MARC Org Codes")
	RoleWorkflowManager = newRole("workflow manager", "Can queue SFTP and scanned issues for processing")
)

// roles is our internal map of string to Role object
var roles = make(map[string]*Role)

// AssignableRoles is a list of roles which can be assigned to a user
var AssignableRoles = []*Role{
	RoleAdmin,
	RoleTitleManager,
	RoleIssueCurator,
	RoleIssueReviewer,
	RoleUserManager,
	RoleMOCManager,
	RoleWorkflowManager,
}

// newRole is internal as the list of roles shouldn't be modified by anything external
func newRole(name, desc string) *Role {
	var r = &Role{Name: name, Desc: oneline(desc)}
	roles[name] = r
	return r
}

// FindRole returns a role looked up by its name, or nil if no such role exists
func FindRole(name string) *Role {
	return roles[name]
}

// Privileges returns which privileges this role has based on our hard-coded lists
func (r *Role) Privileges() []*Privilege {
	var privs []*Privilege
	for _, priv := range Privileges {
		if priv.AllowedBy(r) {
			privs = append(privs, priv)
		}
	}
	return privs
}

// Title returns a slightly nicer string for display
func (r *Role) Title() string {
	// Uppercase all words, and also ensure "MARC" is fully capitalized
	return strings.Title(strings.Replace(r.Name, "marc", "MARC", -1))
}
