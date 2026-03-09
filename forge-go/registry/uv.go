package registry

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// EnsureUV checks if `uvx` is on the PATH. If not, it attempts to download and
// install `uv` (which includes `uvx`) to ~/.forge/bin and prepends it to the PATH
// for the current process.
func EnsureUV() error {
	_, err := exec.LookPath("uvx")
	if err == nil {
		// uvx is already available
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	forgeBin := filepath.Join(homeDir, ".forge", "bin")

	uvxExecutable := "uvx"
	if runtime.GOOS == "windows" {
		uvxExecutable = "uvx.exe"
	}
	uvxPath := filepath.Join(forgeBin, uvxExecutable)
	if _, err := os.Stat(uvxPath); err == nil {
		return addToPath(forgeBin)
	}

	if err := os.MkdirAll(forgeBin, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", forgeBin, err)
	}

	fmt.Printf("uvx not found on PATH. Downloading astral-sh/uv to %s...\n", forgeBin)

	downloadURL, targetDirName, err := getUVReleaseURL()
	if err != nil {
		return err
	}

	isZip := strings.HasSuffix(downloadURL, ".zip")
	if err := downloadAndExtract(downloadURL, targetDirName, forgeBin, isZip); err != nil {
		return err
	}

	fmt.Println("uv downloaded successfully.")

	return addToPath(forgeBin)
}

func addToPath(binDir string) error {
	currentPath := os.Getenv("PATH")
	if !strings.Contains(currentPath, binDir) {
		newPath := fmt.Sprintf("%s%c%s", binDir, os.PathListSeparator, currentPath)
		if err := os.Setenv("PATH", newPath); err != nil {
			return fmt.Errorf("failed to set PATH: %w", err)
		}
	}
	return nil
}

func getUVReleaseURL() (string, string, error) {
	targets := map[string]string{
		"linux/amd64":   "x86_64-unknown-linux-gnu",
		"linux/arm64":   "aarch64-unknown-linux-gnu",
		"darwin/amd64":  "x86_64-apple-darwin",
		"darwin/arm64":  "aarch64-apple-darwin",
		"windows/amd64": "x86_64-pc-windows-msvc",
		"windows/arm64": "aarch64-pc-windows-msvc",
	}

	key := runtime.GOOS + "/" + runtime.GOARCH
	target, ok := targets[key]
	if !ok {
		return "", "", fmt.Errorf("unsupported OS/Arch combination for automatic uv download: %s %s", runtime.GOOS, runtime.GOARCH)
	}

	extension := "tar.gz"
	if runtime.GOOS == "windows" {
		extension = "zip"
	}

	dirName := fmt.Sprintf("uv-%s", target)
	url := fmt.Sprintf("https://github.com/astral-sh/uv/releases/latest/download/%s.%s", dirName, extension)

	return url, dirName, nil
}

func downloadAndExtract(url, targetDirName, destBin string, isZip bool) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download uv from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s when downloading %s", resp.Status, url)
	}

	if isZip {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read zip body: %w", err)
		}
		zr, err := zip.NewReader(bytes.NewReader(bodyBytes), int64(len(bodyBytes)))
		if err != nil {
			return fmt.Errorf("failed to read zip: %w", err)
		}
		found := 0
		for _, file := range zr.File {
			name := filepath.Base(file.Name)
			if name == "uv.exe" || name == "uvx.exe" {
				outPath := filepath.Join(destBin, name)
				outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_RDWR, 0755)
				if err != nil {
					return fmt.Errorf("failed to create binary file %s: %w", outPath, err)
				}
				rc, err := file.Open()
				if err == nil {
					io.Copy(outFile, rc)
					rc.Close()
				}
				outFile.Close()
				found++
			}
		}
		if found < 2 {
			return fmt.Errorf("failed to find uv and uvx binaries in the zip")
		}
		return nil
	}

	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read gzip stream: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	found := 0
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed reading tar: %w", err)
		}

		if header.Typeflag == tar.TypeReg {
			name := filepath.Base(header.Name)
			if name == "uv" || name == "uvx" {
				outPath := filepath.Join(destBin, name)
				outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_RDWR, 0755)
				if err != nil {
					return fmt.Errorf("failed to create binary file %s: %w", outPath, err)
				}
				if _, err := io.Copy(outFile, tr); err != nil {
					outFile.Close()
					return fmt.Errorf("failed to write %s: %w", name, err)
				}
				outFile.Close()
				found++
			}
		}
	}

	if found < 2 {
		return fmt.Errorf("failed to find uv and uvx binaries in the tarball")
	}

	return nil
}
