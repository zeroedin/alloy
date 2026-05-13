package ssr

import "github.com/bytedance/sonic"

// json aliases sonic.ConfigStd for stdlib-compatible JSON behavior with JIT acceleration.
var json = sonic.ConfigStd
