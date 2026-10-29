package github

import (
	"errors"
	"regexp"
	"strings"
)

// ErrInvalidRepo is returned for an owner/repo that is not a GitHub name.
var ErrInvalidRepo = errors.New("not a GitHub owner/repo")

// repoPattern is a GitHub "owner/repo"; it also keeps anything but a name out of an API URL.
var repoPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// ValidRepo reports a GitHub "owner/repo". It also refuses "." and "..", which the pattern admits but which would
// move an API URL's path.
func ValidRepo(ownerRepo string) bool {
	owner, name, _ := strings.Cut(ownerRepo, "/")
	return repoPattern.MatchString(ownerRepo) && strings.Trim(owner, ".") != "" && strings.Trim(name, ".") != ""
}
