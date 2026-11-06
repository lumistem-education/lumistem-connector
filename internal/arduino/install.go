package arduino

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const CLIVersion = "1.5.1"

func cliVersion() string {
	if v := strings.TrimSpace(os.Getenv("ARDUINO_CLI_VERSION")); v != "" {
		return v
	}

	return CLIVersion
}

func cliDownloadBase() string {
	base := strings.TrimSpace(os.Getenv("ARDUINO_CLI_DOWNLOAD_BASE_URL"))
	if base == "" {
		base = "https://downloads.arduino.cc/arduino-cli/"
	}

	return strings.TrimRight(base, "/") + "/"
}

func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return nil, fmt.Errorf("download %s: HTTP %d", url, response.StatusCode)
	}

	const limit = 100 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if len(data) > limit {
		return nil, fmt.Errorf("download too large")
	}

	return data, err
}

func InstallCLI(ctx context.Context, target string) error {
	if _, err := exec.LookPath(target); err == nil {
		return nil
	}
	if !filepath.IsAbs(target) {
		return fmt.Errorf("ARDUINO_CLI_PATH must be an absolute writable path to install Arduino CLI")
	}

	system := map[string]string{"linux": "Linux", "darwin": "macOS", "windows": "Windows"}[runtime.GOOS]
	arch := map[string]string{"amd64": "64bit", "386": "32bit", "arm64": "ARM64", "arm": "ARMv7"}[runtime.GOARCH]
	if system == "" || arch == "" {
		return fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	version := cliVersion()
	extension := "tar.gz"
	binary := "arduino-cli"
	if runtime.GOOS == "windows" {
		extension = "zip"
		binary += ".exe"
	}

	filename := fmt.Sprintf("arduino-cli_%s_%s_%s.%s", version, system, arch, extension)
	base := cliDownloadBase()
	checksumsURL := base + "arduino-cli_" + version + "_checksums.txt"
	archiveURL := base + filename
	if strings.Contains(base, "downloads.arduino.cc") {
		if _, err := download(ctx, checksumsURL); err != nil {
			checksumsURL = fmt.Sprintf("https://github.com/arduino/arduino-cli/releases/download/v%s/arduino-cli_%s_checksums.txt", version, version)
			archiveURL = fmt.Sprintf("https://github.com/arduino/arduino-cli/releases/download/v%s/%s", version, filename)
		}
	}

	checksums, err := download(ctx, checksumsURL)
	if err != nil && !strings.Contains(checksumsURL, "github.com") {
		checksums = nil
	}

	archive, err := download(ctx, archiveURL)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(archive)
	expected := ""

	if len(checksums) > 0 {
		for _, line := range strings.Split(string(checksums), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == filename {
				expected = fields[0]
			}
		}
		if expected == "" || !strings.EqualFold(expected, hex.EncodeToString(hash[:])) {
			return fmt.Errorf("Arduino CLI checksum mismatch")
		}
	}

	var data []byte
	if extension == "zip" {
		reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return err
		}

		for _, file := range reader.File {
			if file.Name == binary {
				r, err := file.Open()
				if err != nil {
					return err
				}
				data, err = io.ReadAll(io.LimitReader(r, 100<<20))
				r.Close()
				if err != nil {
					return err
				}
				break
			}
		}
	} else {
		reader, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			return err
		}

		defer reader.Close()

		tarReader := tar.NewReader(reader)
		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if header.Name == binary && header.Typeflag == tar.TypeReg {
				data, err = io.ReadAll(io.LimitReader(tarReader, 100<<20))
				if err != nil {
					return err
				}
				break
			}
		}
	}

	if len(data) == 0 {
		return fmt.Errorf("Arduino CLI executable missing from archive")
	}

	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(target), "arduino-cli-*")
	if err != nil {
		return err
	}

	defer os.Remove(temp.Name())

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}

	if err := temp.Chmod(0755); err != nil {
		temp.Close()
		return err
	}

	if err := temp.Close(); err != nil {
		return err
	}
	
	return os.Rename(temp.Name(), target)
}
