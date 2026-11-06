package arduino

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func run(ctx context.Context, cliPath, cfgFile, dir string, out io.Writer, args ...string) ([]byte, error) {
	if cliPath == "" {
		cliPath = "arduino-cli"
	}

	fullArgs := []string{}
	if cfgFile != "" {
		fullArgs = append(fullArgs, "--config-file", cfgFile)
	}
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, cliPath, fullArgs...)
	if dir != "" {
		cmd.Dir = dir
	}

	var buf bytes.Buffer
	writers := []io.Writer{&buf}
	if out != nil {
		writers = append(writers, out)
	}
	mw := io.MultiWriter(writers...)
	cmd.Stdout = mw
	var stderr bytes.Buffer
	if out != nil {
		cmd.Stderr = io.MultiWriter(&stderr, out)
	} else {
		cmd.Stderr = &stderr
	}

	err := cmd.Run()
	if err != nil {
		return buf.Bytes(), fmt.Errorf("arduino-cli: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return buf.Bytes(), nil
}

// Compile builds the sketch and writes stdout/stderr to out, returning the combined output.
func Compile(ctx context.Context, cliPath, cfgFile, workspaceDir, fqbn, sketchPath string, out io.Writer) ([]byte, error) {
	absWorkspaceDir, err := filepath.Abs(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace dir: %w", err)
	}
	outputDir := filepath.Join(absWorkspaceDir, "artifact")

	args := []string{
		"compile",
		"--fqbn", fqbn,
		"--output-dir", outputDir,
		"--export-binaries",
		sketchPath,
	}

	return run(ctx, cliPath, cfgFile, absWorkspaceDir, out, args...)
}

// Upload flashes the compiled artifact of a workspace to the given port.
func Upload(ctx context.Context, cliPath, cfgFile, workspaceDir, fqbn, port, sketchPath string, uploadSpeed int, out io.Writer) ([]byte, error) {
	absWorkspaceDir, err := filepath.Abs(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace dir: %w", err)
	}
	inputDir := filepath.Join(absWorkspaceDir, "artifact")

	args := []string{
		"upload",
		"-p", port,
		"--fqbn", fqbn,
		"--input-dir", inputDir,
	}

	if uploadSpeed < 0 || uploadSpeed > 4000000 {
		return nil, fmt.Errorf("invalid upload speed")
	}
	if uploadSpeed > 0 {
		args = append(args, "--upload-property", fmt.Sprintf("upload.speed=%d", uploadSpeed))
	}
	args = append(args, sketchPath)
	return run(ctx, cliPath, cfgFile, absWorkspaceDir, out, args...)
}

func ListPorts(ctx context.Context, cliPath, cfgFile string) ([]byte, error) {
	return run(ctx, cliPath, cfgFile, "", nil, "board", "list", "--json")
}

func ListLibraries(ctx context.Context, cliPath, cfgFile string) ([]byte, error) {
	return run(ctx, cliPath, cfgFile, "", nil, "lib", "list", "--json")
}

func InstallLibrary(ctx context.Context, cliPath, cfgFile, name, version string, out io.Writer) ([]byte, error) {
	spec := name
	if version != "" {
		spec = name + "@" + version
	}

	return run(ctx, cliPath, cfgFile, "", out, "lib", "install", spec)
}

func UninstallLibrary(ctx context.Context, cliPath, cfgFile, name string, out io.Writer) ([]byte, error) {
	return run(ctx, cliPath, cfgFile, "", out, "lib", "uninstall", name)
}

func ListCores(ctx context.Context, cliPath, cfgFile string) ([]byte, error) {
	return run(ctx, cliPath, cfgFile, "", nil, "core", "list", "--json")
}

func IsCoreInstalled(ctx context.Context, cliPath, cfgFile, core string) (bool, error) {
	raw, err := ListCores(ctx, cliPath, cfgFile)
	if err != nil {
		return false, err
	}

	var cores []struct {
		ID               string `json:"id"`
		InstalledVersion string `json:"installed_version"`
		Installed        string `json:"installed"`
	}
	if err := json.Unmarshal(raw, &cores); err != nil {
		var wrapped struct {
			Platforms json.RawMessage `json:"platforms"`
		}
		if err := json.Unmarshal(raw, &wrapped); err != nil {
			return false, err
		}
		if err := json.Unmarshal(wrapped.Platforms, &cores); err != nil {
			return false, err
		}
	}

	for _, c := range cores {
		if c.ID == core && (c.InstalledVersion != "" || c.Installed != "") {
			return true, nil
		}
	}

	return false, nil
}

func InstallCore(ctx context.Context, cliPath, cfgFile, core string, out io.Writer) ([]byte, error) {
	return run(ctx, cliPath, cfgFile, "", out, "core", "install", core)
}

func CoreFromFQBN(fqbn string) string {
	parts := strings.Split(fqbn, ":")
	if len(parts) < 3 {
		return ""
	}

	return parts[0] + ":" + parts[1]
}

func FindArtifactPath(workspaceDir string) (string, error) {
	if workspaceDir == "" {
		return "", nil
	}

	artifactDir := filepath.Join(workspaceDir, "artifact")

	var artifactPath string
	var latestModTime time.Time
	supportedExts := []string{".bin", ".hex", ".uf2"}
	bestScore := -1

	err := filepath.WalkDir(artifactDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return filepath.SkipDir
			}
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		for _, supportedExt := range supportedExts {
			if ext != supportedExt {
				continue
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			score := 1
			if strings.HasSuffix(path, ".merged.bin") {
				score = 3
			} else if strings.HasSuffix(path, ".ino.bin") || strings.HasSuffix(path, ".ino.hex") {
				score = 2
			}
			if score > bestScore || (score == bestScore && info.ModTime().After(latestModTime)) {
				bestScore = score
				artifactPath = path
				latestModTime = info.ModTime()
			}
			break
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	return artifactPath, nil
}

func Command(ctx context.Context, cliPath, cfgFile string, args ...string) ([]byte, error) {
	return run(ctx, cliPath, cfgFile, "", nil, args...)
}
