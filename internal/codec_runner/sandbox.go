package codec_runner

import "github.com/dop251/goja"

// newSandboxRuntime returns a fresh goja.Runtime with no Go-stdlib bindings.
// No `require`, `os`, `fs`, `process`, `setTimeout`, etc. The codec runs
// against ES2020+ builtins only.
//
// Each call returns a NEW runtime — goja.Runtime is NOT goroutine-safe.
// T-07-07-09 mitigation: no shared state across calls.
//
// T-07-07-03 mitigation: goja's default global has standard ES builtins only.
// We explicitly do NOT set require, fs, os, process, setTimeout, setInterval,
// XMLHttpRequest, fetch, WebAssembly, or any host module bindings.
func newSandboxRuntime() *goja.Runtime {
	vm := goja.New()
	vm.SetMaxCallStackSize(maxCallStackFrames)
	// goja's default global has standard ES builtins only — nothing to remove.
	// Explicitly do NOT set: require, fs, os, process, setTimeout, setInterval,
	// XMLHttpRequest, fetch, WebAssembly, or any host module bindings.
	return vm
}
