package gh

import (
	"fmt"
	"strings"
)

// ListNamespaces returns the owners under which the user can have repos: their
// own account login first, then the organizations they belong to, deduplicated
// (order preserved, user first).
func (c *Client) ListNamespaces() ([]string, error) {
	out, err := c.run.Exec("api", "user", "--jq", ".login")
	if err != nil {
		return nil, fmt.Errorf("fetching user login: %w", err)
	}
	user := strings.TrimSpace(string(out))

	out, err = c.run.Exec("api", "user/orgs", "--paginate", "--jq", ".[].login")
	if err != nil {
		return nil, fmt.Errorf("fetching organizations: %w", err)
	}

	var names []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		names = append(names, s)
	}
	add(user)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		add(line)
	}
	return names, nil
}
