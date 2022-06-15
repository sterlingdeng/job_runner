package cgroupz

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func Test_NewCgroup_CreatesFiles(t *testing.T) {
	dir := setupMount(t)
	t.Cleanup(func() {
		cleanUp(dir)
		require.NoDirExists(t, dir)
	})

	type filecontent struct {
		filename string
		content  []byte
	}

	tests := []struct {
		name          string
		limit         ResourceLimit
		expectedFiles []filecontent
	}{
		{
			name:  "test cpu",
			limit: ResourceLimit{CpuWeight: 99},
			expectedFiles: []filecontent{
				{
					filename: "cpu.weight",
					content:  []byte("99"),
				},
			},
		},
		{
			name:  "test memory",
			limit: ResourceLimit{MaxMem: 28},
			expectedFiles: []filecontent{
				{
					filename: "memory.max",
					content:  []byte("28"),
				},
			},
		},
		{
			name: "test io",
			limit: ResourceLimit{MaxIO: &IOLimit{
				MaxIO: 22,
				Maj:   8,
				Min:   6,
			}},
			expectedFiles: []filecontent{
				{
					filename: "io.max",
					content:  []byte("8:6 wiops=22"),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			randstr := uuid.New().String()
			_, err := New(randstr, dir, test.limit)
			require.NoError(t, err)

			for _, c := range test.expectedFiles {
				filepath := filepath.Join(dir, randstr, c.filename)
				require.FileExists(t, filepath)
				contents, err := ioutil.ReadFile(filepath)
				require.NoError(t, err)
				require.Equal(t, c.content, contents)
			}
		})
	}
}

func setupMount(t *testing.T) string {
	os.TempDir()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("cgroup_test_%s", uuid.New().String()))
	err := os.Mkdir(dir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "cgroup.subtree_control"), []byte{}, 0644)
	require.NoError(t, err)
	return dir
}
