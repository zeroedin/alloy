package fileutil_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/fileutil"
)

var _ = Describe("CopyFile", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "copyfile-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	It("returns os.ErrNotExist when the source file does not exist", func() {
		err := fileutil.CopyFile("/nonexistent/path/file.txt", filepath.Join(tmpDir, "dest.txt"))
		Expect(err).To(HaveOccurred())
		Expect(os.IsNotExist(err)).To(BeTrue(),
			"CopyFile must return an error wrapping os.ErrNotExist when the source "+
				"file does not exist — callers rely on this to distinguish missing "+
				"files from other copy failures (issue #782)")
	})

	It("copies a file preserving permissions", func() {
		srcPath := filepath.Join(tmpDir, "source.txt")
		dstPath := filepath.Join(tmpDir, "subdir", "dest.txt")
		Expect(os.WriteFile(srcPath, []byte("hello"), 0644)).To(Succeed())

		Expect(fileutil.CopyFile(srcPath, dstPath)).To(Succeed())

		content, err := os.ReadFile(dstPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal("hello"))

		srcInfo, _ := os.Stat(srcPath)
		dstInfo, _ := os.Stat(dstPath)
		Expect(dstInfo.Mode()).To(Equal(srcInfo.Mode()),
			"CopyFile must preserve the source file's permissions")
	})

	It("returns os.ErrNotExist when the source file was deleted before copy", func() {
		srcPath := filepath.Join(tmpDir, "transient.txt")
		dstPath := filepath.Join(tmpDir, "dest.txt")
		Expect(os.WriteFile(srcPath, []byte("temp"), 0644)).To(Succeed())
		Expect(os.Remove(srcPath)).To(Succeed())

		err := fileutil.CopyFile(srcPath, dstPath)
		Expect(err).To(HaveOccurred())
		Expect(os.IsNotExist(err)).To(BeTrue(),
			"CopyFile must return os.ErrNotExist when the source file vanishes — "+
				"the dev server watcher relies on this to silently skip transient files")
	})
})
