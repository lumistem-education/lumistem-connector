package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ServerMode string

const (
	ModePublic ServerMode = "public"
	ModeLocal  ServerMode = "local"
)

func (m ServerMode) IsLocal() bool {
	return m == ModeLocal
}

func parseServerMode(raw string) (ServerMode, error) {
	switch ServerMode(strings.ToLower(strings.TrimSpace(raw))) {
	case ModePublic:
		return ModePublic, nil
	case ModeLocal:
		return ModeLocal, nil
	default:
		return "", fmt.Errorf("invalid SERVER_MODE %q: expected %q or %q", raw, ModePublic, ModeLocal)
	}
}

const LocalPort = "8765"

type Config struct {
	ArduinoMu          sync.RWMutex
	Mode               ServerMode
	Port               string
	WorkspaceDir       string
	CompileTimeout     time.Duration
	UploadTimeout      time.Duration
	CoreInstallTimeout time.Duration
	LibraryOpTimeout   time.Duration
	MaxConcurrentJobs  int
	JWTSecret          string
	ArduinoCLIPath     string
	ArduinoConfigFile  string
	ArduinoDataDir     string
}

func Load() (*Config, error) {
	loadDotEnv()
	mode, err := parseServerMode(getenv("SERVER_MODE", string(ModeLocal)))
	if err != nil {
		return nil, err
	}
	port := getenv("PORT", "8080")
	appDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	appDir = filepath.Join(appDir, "LumiSTEM", "compiler")
	if mode.IsLocal() {
		port = LocalPort
	}
	baseDir := filepath.Clean(getenv("LUMISTEM_ENV_DIR", "."))
	workspace := resolveRelativePath(getenv("WORKSPACE_DIR", filepath.Join(appDir, "workspace")), baseDir)
	timeoutSec := atoiDefault(getenv("COMPILE_TIMEOUT_SEC", "60"), 60)
	uploadTimeoutSec := atoiDefault(getenv("UPLOAD_TIMEOUT_SEC", "180"), 180)
	coreInstallSec := atoiDefault(getenv("CORE_INSTALL_TIMEOUT_SEC", "600"), 600)
	libraryOpSec := atoiDefault(getenv("LIBRARY_OP_TIMEOUT_SEC", "300"), 300)
	maxJobs := atoiDefault(getenv("MAX_CONCURRENT_JOBS", "2"), 2)
	jwtSecret := getenv("JWT_SECRET", "this-is-a-very-long-secret-key-for-jwt-signing-please-change-it-for-production")
	binaryName := "arduino-cli"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	defaultCLI := filepath.Join(appDir, "bin", binaryName)
	if executable, err := os.Executable(); err == nil {
		bundled := filepath.Join(filepath.Dir(executable), "bin", binaryName)
		if _, err := os.Stat(bundled); err == nil {
			defaultCLI = bundled
		}
	}
	if _, err := os.Stat(defaultCLI); err != nil {
		if installed, err := exec.LookPath(binaryName); err == nil {
			defaultCLI = installed
		}
	}
	arduinoCli := resolveRelativePath(getenv("ARDUINO_CLI_PATH", defaultCLI), baseDir)
	arduinoCfg := resolveRelativePath(getenv("ARDUINO_CONFIG_FILE", filepath.Join(appDir, "arduino-cli.yaml")), baseDir)
	arduinoData := resolveRelativePath(getenv("ARDUINO_DATA_DIR", filepath.Join(appDir, "data")), baseDir)

	if abs, err := filepath.Abs(arduinoCfg); err == nil {
		arduinoCfg = abs
	}
	if abs, err := filepath.Abs(arduinoData); err == nil {
		arduinoData = abs
	}

	if len(jwtSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must contain at least 32 characters")
	}

	cfg := &Config{
		Mode:               mode,
		Port:               port,
		WorkspaceDir:       workspace,
		CompileTimeout:     time.Duration(timeoutSec) * time.Second,
		UploadTimeout:      time.Duration(uploadTimeoutSec) * time.Second,
		CoreInstallTimeout: time.Duration(coreInstallSec) * time.Second,
		LibraryOpTimeout:   time.Duration(libraryOpSec) * time.Second,
		MaxConcurrentJobs:  maxJobs,
		JWTSecret:          jwtSecret,
		ArduinoCLIPath:     arduinoCli,
		ArduinoConfigFile:  arduinoCfg,
		ArduinoDataDir:     arduinoData,
	}

	if cfg.MaxConcurrentJobs <= 0 {
		return nil, fmt.Errorf("MAX_CONCURRENT_JOBS must be > 0")
	}
	if cfg.CompileTimeout <= 0 {
		return nil, fmt.Errorf("COMPILE_TIMEOUT_SEC must be > 0")
	}
	if cfg.UploadTimeout <= 0 {
		return nil, fmt.Errorf("UPLOAD_TIMEOUT_SEC must be > 0")
	}
	if cfg.CoreInstallTimeout <= 0 {
		return nil, fmt.Errorf("CORE_INSTALL_TIMEOUT_SEC must be > 0")
	}
	if cfg.LibraryOpTimeout <= 0 {
		return nil, fmt.Errorf("LIBRARY_OP_TIMEOUT_SEC must be > 0")
	}

	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func loadDotEnv() {
	candidates := make([]string, 0, 6)
	if envFile := strings.TrimSpace(os.Getenv("ENV_FILE")); envFile != "" {
		candidates = append(candidates, envFile)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, ".env"))
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), ".env"))
	}
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		candidate = filepath.Clean(candidate)
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if err := loadEnvFile(candidate); err == nil {
			if abs, err := filepath.Abs(candidate); err == nil {
				_ = os.Setenv("LUMISTEM_ENV_DIR", filepath.Dir(abs))
			}
		}
	}
}

func loadEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if _, ok := os.LookupEnv(key); ok {
			continue
		}
		value = strings.Trim(value, "\"'")
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
}

func resolveRelativePath(value, baseDir string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	if baseDir == "" || baseDir == "." {
		if abs, err := filepath.Abs(value); err == nil {
			return abs
		}
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(baseDir, value))
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}
