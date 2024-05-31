package download

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChecksumValidate(t *testing.T) {
	tests := []struct {
		testName      string
		filename      string
		checksum      string
		expectedError error
	}{
		{
			"no checksum prefix defined, default to md5",
			"foo.bin",
			"1649cff06611a6025da3dd511a97fb43", // file contents 'BLOB'
			nil,
		},
		{
			"md5 prefix defined",
			"foo.bin",
			"md5sum:1649cff06611a6025da3dd511a97fb43", // file contents 'BLOB'
			nil,
		},
		{
			"checksum is wrong",
			"foo.bin",
			"md5sum:bee8af7a84cb640cff90cf31fbf56950",
			ErrChecksum,
		},
		{
			"too many colons",
			"foo.bin",
			"md5sum:1649:cff06611a6025da3dd511a97fb43",
			ErrFormat,
		},
		{
			"unsupported digest format",
			"foo.bin",
			"vince:some-digest-format",
			ErrFormat,
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

			err = ChecksumValidate(binPath, tt.checksum)
			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
				return
			}

			assert.Nil(t, err)
		})
	}

}
