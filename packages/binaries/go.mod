module github.com/swayyaam/lasso/packages/binaries

go 1.26.0

require (
	github.com/ProtonMail/go-crypto v1.4.1
	github.com/swayyaam/lasso/packages/ghrelease v0.0.0
)

require (
	github.com/cloudflare/circl v1.6.5 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/swayyaam/lasso/packages/ghrelease => ../ghrelease
