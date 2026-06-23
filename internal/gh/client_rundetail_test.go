package gh

import "testing"

func TestGetRunParses(t *testing.T) {
	const out = `{
	  "status":"completed","conclusion":"failure",
	  "jobs":[
	    {"databaseId":900,"name":"build","status":"completed","conclusion":"failure",
	     "steps":[
	       {"number":1,"name":"checkout","status":"completed","conclusion":"success"},
	       {"number":2,"name":"test","status":"completed","conclusion":"failure"}
	     ]}
	  ]
	}`
	f := (&fakeRunner{}).push(out, nil)
	c := NewClient(f)

	rd, err := c.GetRun(RepoRef{"o", "r"}, 900)
	if err != nil {
		t.Fatal(err)
	}
	if rd.Status != "completed" || rd.Conclusion != "failure" {
		t.Errorf("run = %+v", rd.Run)
	}
	if len(rd.Jobs) != 1 || rd.Jobs[0].Name != "build" {
		t.Fatalf("jobs = %+v", rd.Jobs)
	}
	if len(rd.Jobs[0].Steps) != 2 || rd.Jobs[0].Steps[1].Conclusion != "failure" {
		t.Errorf("steps = %+v", rd.Jobs[0].Steps)
	}
	got := f.lastCall()
	if got[0] != "run" || got[1] != "view" || got[2] != "900" {
		t.Errorf("argv = %v", got)
	}
}
