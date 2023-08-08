package outofband

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/errors"
)

var (
	downloadRetryDelay    = 4 * time.Second
	downloadClientTimeout = 60 * time.Second

	ErrDownload = errors.New("error downloading file")
	ErrChecksum = errors.New("error validating file checksum")
)

// download fetches the file into dst
func download(ctx context.Context, fileURL, dst string) error {
	// create file
	fileHandle, err := os.Create(dst)
	if err != nil {
		return err
	}

	defer fileHandle.Close()

	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, http.NoBody)
	if err != nil {
		return err
	}

	requestRetryable, err := retryablehttp.FromRequest(req)
	if err != nil {
		return err
	}

	client := retryablehttp.NewClient()
	client.RetryWaitMin = downloadRetryDelay
	client.Logger = nil
	client.HTTPClient.Timeout = downloadClientTimeout

	resp, err := client.Do(requestRetryable)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return errors.Wrap(ErrDownload, fmt.Sprintf("URL: %s, status code %s", fileURL, resp.Status))
	}

	_, err = io.Copy(fileHandle, resp.Body)

	return err
}

func checksumValidate(filename, checksum string) error {
	errChecksumPrefix := errors.New("checksum prefix error")

	// no checksum prefix, default to md5sum
	if !strings.Contains(checksum, ":") {
		return checksumValidateMD5(filename, checksum)
	}

	parts := strings.Split(checksum, ":")
	if len(parts) < 2 {
		return errors.Wrap(errChecksumPrefix, checksum)
	}

	switch parts[0] {
	case "md5sum":
		return checksumValidateMD5(filename, checksum)
	default:
		return errors.Wrap(errChecksumPrefix, "unsupported checksum: "+parts[0])
	}
}

// TODO: firmware-syncer needs to prefix firmware checksums values with the type of checksum
// so consumers can validate it accordingly
func checksumValidateMD5(filename, checksum string) error {
	var err error

	expectedChecksum := []byte(checksum)

	if filename == "" {
		return errors.Wrap(ErrChecksum, "expected a filename to validate checksum")
	}

	// calculate checksum for filename
	f, err := os.Open(filename)
	if err != nil {
		return errors.Wrap(ErrChecksum, err.Error()+filename)
	}
	defer f.Close()

	h := md5.New()

	// TODO(joel) - wrap this within a context
	if _, err := io.Copy(h, f); err != nil {
		return errors.Wrap(ErrChecksum, err.Error())
	}

	calculatedChecksum := fmt.Sprintf("%x", h.Sum(nil))
	if !bytes.Equal(expectedChecksum, []byte(calculatedChecksum)) {
		errMsg := fmt.Sprintf(
			"filename: %s expected: %s, got: %s",
			filename,
			string(expectedChecksum),
			string(calculatedChecksum),
		)

		return errors.Wrap(ErrChecksum, errMsg)
	}

	return nil
}
