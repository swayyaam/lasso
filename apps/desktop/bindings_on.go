//go:build bindings

package main

// generatingBindings is true in the build `wails generate module` runs to read
// the bound methods. That build must not take the one-copy lock: with Lasso
// open, it would hand over to the running copy and exit before writing a
// binding, and the generator reports success anyway.
const generatingBindings = true
