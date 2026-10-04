package demo

import "encoding/json"

// Result is what RunJSON answers: the table, or the error that stopped it.
type Result struct {
	Table
	Error string `json:"error,omitempty"`
}

// RunJSON runs a spec given as JSON and answers the Result as JSON. It's the boundary the in-page
// editor crosses (docsite/wasm), so the browser runs an edited example through the same Run as the
// build and the tests. A spec that doesn't decode answers an error, never a panic.
func RunJSON(spec string) string {
	var s Spec
	var r Result
	if err := json.Unmarshal([]byte(spec), &s); err != nil {
		r.Error = "demo: the spec isn't valid JSON: " + err.Error()
	} else if t, err := Run(s); err != nil {
		r.Error = err.Error()
	} else {
		r.Table = t
	}
	out, _ := json.Marshal(r)
	return string(out)
}
