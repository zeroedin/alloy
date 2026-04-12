package template_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stretchr/testify/mock"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

// MockTemplateEngine implements TemplateEngine for testing.
type MockTemplateEngine struct {
	mock.Mock
}

func (m *MockTemplateEngine) Parse(name string, content []byte) (tmpl.Template, error) {
	args := m.Called(name, content)
	if t := args.Get(0); t != nil {
		return t.(tmpl.Template), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockTemplateEngine) AddFilter(name string, fn tmpl.FilterFunc) error {
	args := m.Called(name, fn)
	return args.Error(0)
}

func (m *MockTemplateEngine) AddTag(name string, fn tmpl.TagFunc) error {
	args := m.Called(name, fn)
	return args.Error(0)
}

// MockTemplate implements Template for testing.
type MockTemplate struct {
	mock.Mock
}

func (m *MockTemplate) Render(ctx map[string]interface{}) ([]byte, error) {
	args := m.Called(ctx)
	if b := args.Get(0); b != nil {
		return b.([]byte), args.Error(1)
	}
	return nil, args.Error(1)
}

var _ = Describe("Mock-based engine tests", func() {
	It("TemplateEngine.Parse can be mocked to return a template", func() {
		engine := new(MockTemplateEngine)
		mockTpl := new(MockTemplate)

		engine.On("Parse", "test", []byte("hello")).Return(mockTpl, nil)
		mockTpl.On("Render", mock.Anything).Return([]byte("rendered"), nil)

		tpl, err := engine.Parse("test", []byte("hello"))
		Expect(err).NotTo(HaveOccurred())
		Expect(tpl).NotTo(BeNil())

		result, err := tpl.Render(map[string]interface{}{"key": "value"})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(result)).To(Equal("rendered"))

		engine.AssertExpectations(GinkgoT())
		mockTpl.AssertExpectations(GinkgoT())

		// Verify a real LiquidEngine can also parse and render
		realEngine := tmpl.NewLiquidEngine()
		Expect(realEngine).NotTo(BeNil(), "NewLiquidEngine must return a non-nil engine")
		realTpl, realErr := realEngine.Parse("test", []byte("hello"))
		Expect(realErr).NotTo(HaveOccurred(),
			"real engine parse must not error")
		Expect(realTpl).NotTo(BeNil(),
			"real engine parse must return a template")
		realResult, renderErr := realTpl.Render(map[string]interface{}{})
		Expect(renderErr).NotTo(HaveOccurred(),
			"real engine render must not error")
		Expect(string(realResult)).To(ContainSubstring("hello"),
			"real engine parse+render round-trip must produce output")
	})

	It("TemplateEngine.AddFilter can be mocked", func() {
		engine := new(MockTemplateEngine)
		engine.On("AddFilter", "upcase", mock.AnythingOfType("template.FilterFunc")).Return(nil)

		err := engine.AddFilter("upcase", func(input interface{}, args ...interface{}) interface{} {
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		engine.AssertExpectations(GinkgoT())

		// Verify AddFilter on a real LiquidEngine works (not just mock)
		realEngine := tmpl.NewLiquidEngine()
		Expect(realEngine).NotTo(BeNil(), "NewLiquidEngine must return a non-nil engine")
		realErr := realEngine.AddFilter("upcase", func(input interface{}, args ...interface{}) interface{} {
			return nil
		})
		Expect(realErr).NotTo(HaveOccurred(),
			"AddFilter on real engine must not return error")
	})
})
