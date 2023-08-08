package outofband

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_checksumValidate(t *testing.T) {
	tests := []struct {
		testName      string
		filename      string
		checksum      string
		expectedError string
	}{
		{
			"no checksum prefix defined, default to md5",
			"foo.bin",
			"1649cff06611a6025da3dd511a97fb43", // file contents 'BLOB'
			"",
		},
		{
			"md5 prefix defined",
			"foo.bin",
			"md5sum:1649cff06611a6025da3dd511a97fb43", // file contents 'BLOB'
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			tmpdir := t.TempDir()
			binPath := filepath.Join(tmpdir, tt.filename)
			err := os.WriteFile(binPath, []byte(`BLOB`), 0600)
			if err != nil {
				t.Fatal(err)
			}

			defer os.Remove(binPath)

			err = checksumValidate(binPath, tt.checksum)
			if tt.expectedError != "" {
				assert.Equal(t, tt.expectedError, err)
				return
			}

			assert.Nil(t, err)
		})
	}

}
