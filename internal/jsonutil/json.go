package jsonutil

import "github.com/bytedance/sonic"

// JSON is the project-wide JSON codec. All packages use this via
// `var jsonCodec = jsonutil.JSON` so there is a single point of change
// if we swap JSON libraries.
var JSON = sonic.ConfigStd
