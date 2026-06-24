package gh

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ListBranches returns branch names for repo.
func (c *Client) ListBranches(repo RepoRef) ([]string, error) {
	out, err := c.run.Exec("api",
		fmt.Sprintf("repos/%s/%s/branches", repo.Owner, repo.Name),
		"--paginate", "--jq", ".[].name")
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			branches = append(branches, s)
		}
	}
	return branches, nil
}

type jsonRepo struct {
	NameWithOwner string `json:"nameWithOwner"`
}

// ListOrgRepos lists repositories owned by org/user.
func (c *Client) ListOrgRepos(org string) ([]RepoRef, error) {
	out, err := c.run.Exec("repo", "list", org, "--json", "nameWithOwner", "--limit", "200")
	if err != nil {
		return nil, err
	}
	var raw []jsonRepo
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing repo list: %w", err)
	}
	repos := make([]RepoRef, 0, len(raw))
	for _, j := range raw {
		r, err := ParseRepoRef(j.NameWithOwner)
		if err != nil {
			continue
		}
		repos = append(repos, r)
	}
	return repos, nil
}
