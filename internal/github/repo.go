package github

import (
	"fmt"
	"strings"
)

// Repo is a GitHub repository. An empty Host means the default host of gh.
type Repo struct {
	Host  string
	Owner string
	Name  string
}

// ParseRepo reads OWNER/NAME or HOST/OWNER/NAME, the same forms as gh -R.
func ParseRepo(s string) (Repo, error) {
	parts := strings.Split(s, "/")

	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return Repo{}, badRepo(s)
		}
	}

	switch len(parts) {
	case 2:
		return Repo{Owner: parts[0], Name: parts[1]}, nil

	case 3:
		return Repo{Host: parts[0], Owner: parts[1], Name: parts[2]}, nil

	default:
		return Repo{}, badRepo(s)
	}
}

func badRepo(s string) error {
	return fmt.Errorf("repository %q must be OWNER/NAME or HOST/OWNER/NAME", s)
}

// FullName is OWNER/NAME.
func (r Repo) FullName() string {
	return r.Owner + "/" + r.Name
}

// flag is the value for gh -R.
func (r Repo) flag() string {
	if r.Host == "" {
		return r.FullName()
	}

	return r.Host + "/" + r.FullName()
}
