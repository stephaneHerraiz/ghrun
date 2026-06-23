package gh

import "testing"

func TestParseRepoRef(t *testing.T) {
	r, err := ParseRepoRef("owner/name")
	if err != nil {
		t.Fatal(err)
	}
	if r.Owner != "owner" || r.Name != "name" {
		t.Errorf("got %+v", r)
	}
	if r.String() != "owner/name" {
		t.Errorf("String() = %q", r.String())
	}
}

func TestParseRepoRefInvalid(t *testing.T) {
	for _, s := range []string{"", "noslash", "a/b/c", "/x", "x/"} {
		if _, err := ParseRepoRef(s); err == nil {
			t.Errorf("ParseRepoRef(%q) expected error", s)
		}
	}
}

func TestRunActive(t *testing.T) {
	if !(Run{Status: "in_progress"}).Active() {
		t.Error("in_progress should be active")
	}
	if !(Run{Status: "queued"}).Active() {
		t.Error("queued should be active")
	}
	if (Run{Status: "completed"}).Active() {
		t.Error("completed should not be active")
	}
}
