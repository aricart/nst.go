package nst

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDir a directory  with some simple powers
type TestDir struct {
	t   testing.TB
	Dir string
}

func NewTestDir(t testing.TB, dir string, pattern string) *TestDir {
	if dir == "" {
		dir = os.TempDir()
	}
	dirPath, err := os.MkdirTemp(dir, pattern)
	require.NoError(t, err)
	return &TestDir{t: t, Dir: dirPath}
}

func (td *TestDir) String() string {
	return td.Dir
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
