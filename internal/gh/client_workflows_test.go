package gh

import "testing"

func TestListWorkflowsParses(t *testing.T) {
	const out = `[
	  {"id":1234,"name":"CI","path":".github/workflows/ci.yml","state":"active"},
	  {"id":5678,"name":"Deploy","path":".github/workflows/deploy.yml","state":"active"}
	]`
	f := (&fakeRunner{}).push(out, nil)
	c := NewClient(f)

	wfs, err := c.ListWorkflows(RepoRef{"o", "r"})
	if err != nil {
		t.Fatal(err)
	}
	if len(wfs) != 2 || wfs[0].ID != 1234 || wfs[1].Path != ".github/workflows/deploy.yml" {
		t.Fatalf("got %+v", wfs)
	}
}
