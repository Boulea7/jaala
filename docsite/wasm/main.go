//go:build js && wasm

// Command wasm is jaala compiled for the browser, for the docsite's in-page editor. It registers
// jaalaRun(specJSON) -> resultJSON on the page (demo.RunJSON) and then waits, since a Go program
// compiled to wasm serves calls only while main is running.
package main

import (
	"syscall/js"

	"github.com/panyam/jaala/docsite/demo"
)

func main() {
	js.Global().Set("jaalaRun", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 1 {
			return `{"error": "jaalaRun takes one spec, as JSON"}`
		}
		return demo.RunJSON(args[0].String())
	}))
	select {}
}
