package release

import (
	_ "embed" // this needs go 1.16+
)

//go:embed version
var CAPVCDVersion string
