package outofband

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/errors"
)

var (
	downloadRetryDelay = 4 * time.Second

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

	resp, err := client.Do(requestRetryable)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return errors.Wrap(ErrDownload, fmt.Sprintf("status code %s", resp.Status))
	}

	_, err = io.Copy(fileHandle, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func checksumValidateSHA256(filename, checksum string) error {
	var expectedChecksum []byte

	var err error

	if filename == "" {
		return errors.Wrap(ErrChecksum, "expected a filename to validate checksum")
	}

	// calculate checksum for filename
	f, err := os.Open(filename)
	if err != nil {
		return errors.Wrap(ErrChecksum, err.Error()+filename)
	}
	defer f.Close()

	h := sha256.New()

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
