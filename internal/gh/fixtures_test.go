package gh

import "fmt"

// fakeRunner records Exec calls and returns queued responses in order.
type fakeRunner struct {
	calls     [][]string
	responses []fakeResponse
	idx       int
}

type fakeResponse struct {
	out []byte
	err error
}

func (f *fakeRunner) push(out string, err error) *fakeRunner {
	f.responses = append(f.responses, fakeResponse{out: []byte(out), err: err})
	return f
}

func (f *fakeRunner) Exec(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if f.idx >= len(f.responses) {
		return nil, fmt.Errorf("fakeRunner: no response queued for call %d: %v", f.idx, args)
	}
	r := f.responses[f.idx]
	f.idx++
	return r.out, r.err
}

// lastCall returns the most recent argv passed to Exec.
func (f *fakeRunner) lastCall() []string {
	if len(f.calls) == 0 {
		return nil
	}
	return f.calls[len(f.calls)-1]
}
