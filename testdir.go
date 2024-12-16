package nst

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDir a directory  with some simple powers
type TestDir struct {
	t   *testing.T
	Dir string
}

func NewTestDir(t *testing.T, dir string, pattern string) *TestDir {
	dirPath, err := os.MkdirTemp(dir, pattern)
	require.NoError(t, err)
	return &TestDir{t: t, Dir: dirPath}
}

func (td *TestDir) Cleanup() {
	if td.t.Failed() {
		td.t.Logf("test Dir location: %v", td)
	} else {
		_ = os.RemoveAll(td.Dir)
	}
}

func (td *TestDir) WriteFile(name string, conf []byte) string {
	fp := path.Join(td.Dir, name)
	require.NoError(td.t, os.WriteFile(fp, conf, 0o644))
	return fp
}

func (td *TestDir) ReadFile(name string) []byte {
	data, err := os.ReadFile(path.Join(td.Dir, name))
	require.NoError(td.t, err)
	return data
}
