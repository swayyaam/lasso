module github.com/swayyaam/lasso/packages/doctor

go 1.26.0

require (
	github.com/swayyaam/lasso/packages/binaries v0.0.0
	github.com/swayyaam/lasso/packages/core v0.0.0
)

require golang.org/x/image v0.46.0 // indirect

// The modules in this repo are never published, so these resolve through the
// filesystem rather than a version.
replace github.com/swayyaam/lasso/packages/core => ../core

replace github.com/swayyaam/lasso/packages/binaries => ../binaries
