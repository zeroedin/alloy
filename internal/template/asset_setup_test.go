package template_test

import (
	. "github.com/onsi/ginkgo/v2"

	tmpl "github.com/zeroedin/alloy/internal/template"
)

var _ = BeforeSuite(func() {
	tmpl.RegisterAssetFilters("testdata")
})
