package gh

import "testing"

type errString string

func (e errString) Error() string { return string(e) }

func TestListNamespaces(t *testing.T) {
	// First Exec → `gh api user --jq .login` → the user login.
	// Second Exec → `gh api user/orgs --paginate --jq .[].login` → org logins (newline list).
	f := (&fakeRunner{}).push("alice\n", nil).push("acme\nwidgets\n", nil)
	c := NewClient(f)

	ns, err := c.ListNamespaces()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alice", "acme", "widgets"}
	if len(ns) != len(want) {
		t.Fatalf("namespaces = %v, want %v", ns, want)
	}
	for i := range want {
		if ns[i] != want[i] {
			t.Fatalf("namespaces[%d] = %q, want %q (full: %v)", i, ns[i], want[i], ns)
		}
	}
	// Verify the two gh calls.
	if len(f.calls) != 2 {
		t.Fatalf("expected 2 gh calls, got %d: %v", len(f.calls), f.calls)
	}
	if f.calls[0][0] != "api" || f.calls[0][1] != "user" {
		t.Errorf("call 0 = %v, want [api user --jq .login]", f.calls[0])
	}
	if f.calls[1][0] != "api" || f.calls[1][1] != "user/orgs" {
		t.Errorf("call 1 = %v, want [api user/orgs ...]", f.calls[1])
	}
}

func TestListNamespacesDedupesUserInOrgs(t *testing.T) {
	// If the user login also appears in the orgs list, it must not be duplicated.
	f := (&fakeRunner{}).push("alice\n", nil).push("alice\nacme\n", nil)
	c := NewClient(f)
	ns, err := c.ListNamespaces()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alice", "acme"}
	if len(ns) != 2 || ns[0] != "alice" || ns[1] != "acme" {
		t.Fatalf("namespaces = %v, want %v", ns, want)
	}
}

func TestListNamespacesUserError(t *testing.T) {
	f := (&fakeRunner{}).push("", errString("boom"))
	c := NewClient(f)
	if _, err := c.ListNamespaces(); err == nil {
		t.Fatal("expected error when `gh api user` fails")
	}
}
