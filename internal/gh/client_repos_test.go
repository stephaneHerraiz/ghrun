package gh

import "testing"

func TestListBranchesParses(t *testing.T) {
	f := (&fakeRunner{}).push("main\ndevelop\nfeature/x\n", nil)
	c := NewClient(f)
	br, err := c.ListBranches(RepoRef{"o", "r"})
	if err != nil {
		t.Fatal(err)
	}
	if len(br) != 3 || br[0] != "main" || br[2] != "feature/x" {
		t.Fatalf("branches = %v", br)
	}
}

func TestListOrgReposParses(t *testing.T) {
	const out = `[{"nameWithOwner":"acme/a"},{"nameWithOwner":"acme/b"}]`
	f := (&fakeRunner{}).push(out, nil)
	c := NewClient(f)
	repos, err := c.ListOrgRepos("acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 || repos[1].String() != "acme/b" {
		t.Fatalf("repos = %v", repos)
	}
}
