package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"lumistem/compiler-services/internal/arduino"
	"lumistem/compiler-services/internal/config"
	"lumistem/compiler-services/internal/events"
	"lumistem/compiler-services/internal/handler"
	"lumistem/compiler-services/internal/job"
	"lumistem/compiler-services/internal/logs"
	"lumistem/compiler-services/internal/serial"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		logs.Fatal("Load config: %v", err)
	}

	jm := job.NewManager(cfg.WorkspaceDir)
	hub := events.NewHub()

	if err := ensureDir(cfg.WorkspaceDir); err != nil {
		logs.Fatal("Create workspace: %v", err)
	}
	if err := ensureDir(cfg.ArduinoDataDir); err != nil {
		logs.Fatal("Create Arduino data directory: %v", err)
	}

	go func() {
		bootstrapCtx, cancel := context.WithTimeout(context.Background(), cfg.CoreInstallTimeout)
		defer cancel()
		if err := bootstrapArduino(bootstrapCtx, cfg); err != nil {
			logs.Error("Initialize Arduino tools: %v", err)
			return
		}
	}()

	addr := fmt.Sprintf(":%s", cfg.Port)
	if cfg.Mode.IsLocal() {
		addr = "127.0.0.1:" + config.LocalPort
	}
	logs.Info("Listening on %s (mode=%s)", addr, cfg.Mode)

	var serverHandler http.Handler = newRouter(cfg, jm, hub)
	if cfg.Mode.IsLocal() {
		serverHandler = cors(serverHandler)
	}
	if err := http.ListenAndServe(addr, serverHandler); err != nil {
		logs.Fatal("Server stopped: %v", err)
	}
}

func newRouter(cfg *config.Config, jm *job.Manager, hub *events.Hub) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/jobs/compile", handler.NewCompileHandler(cfg, jm))
	mux.HandleFunc("GET /api/jobs/{jobID}/artifact", handler.NewDownloadArtifactHandler(cfg, jm))
	mux.HandleFunc("DELETE /api/jobs/{jobID}/workspace", handler.NewDeleteWorkspaceHandler(cfg, jm))
	mux.HandleFunc("GET /api/libs", handler.NewListLibrariesHandler(cfg))

	mux.HandleFunc("/compile/", events.NewCompileHandler(cfg, hub, jm))

	mux.HandleFunc("GET /api/health", handler.NewHealthHandler(cfg))
	if cfg.Mode.IsLocal() {
		handler.RegisterToolsRoutes(mux, cfg)
		sm := serial.NewManager(cfg.ArduinoCLIPath, cfg.ArduinoConfigFile)

		mux.HandleFunc("GET /api/ports", handler.NewListPortsHandler(cfg))
		mux.HandleFunc("POST /api/libs/install", handler.NewInstallLibraryHandler(cfg))
		mux.HandleFunc("DELETE /api/libs/{name}", handler.NewUninstallLibraryHandler(cfg))

		mux.HandleFunc("POST /api/serial/open", handler.NewOpenSerialHandler(sm))
		mux.HandleFunc("POST /api/serial/{sessionID}/write", handler.NewWriteSerialHandler(sm))
		mux.HandleFunc("DELETE /api/serial/{sessionID}", handler.NewCloseSerialHandler(sm))
		mux.HandleFunc("GET /serial/{sessionID}/read", handler.NewReadSerialHandler(sm))

		mux.HandleFunc("/flash/", events.NewFlashHandler(cfg, hub, jm))
	}

	return mux
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func bootstrapArduino(ctx context.Context, cfg *config.Config) error {
	if err := arduino.InstallCLI(ctx, cfg.ArduinoCLIPath); err != nil {
		return fmt.Errorf("install arduino-cli: %w", err)
	}
	if _, err := os.Stat(cfg.ArduinoConfigFile); os.IsNotExist(err) {
		if err := os.MkdirAll(cfg.ArduinoDataDir, 0o755); err != nil {
			return fmt.Errorf("create arduino data dir: %w", err)
		}
		if err := os.MkdirAll(cfg.ArduinoDataDir+"/staging", 0o755); err != nil {
			return fmt.Errorf("create arduino staging dir: %w", err)
		}
		if err := os.MkdirAll(cfg.ArduinoDataDir+"/user", 0o755); err != nil {
			return fmt.Errorf("create arduino user dir: %w", err)
		}
		contents := []byte(fmt.Sprintf(`directories:
  data: %s
  downloads: %s
  user: %s
board_manager:
  additional_urls:
    - https://espressif.github.io/arduino-esp32/package_esp32_index.json
`, cfg.ArduinoDataDir, cfg.ArduinoDataDir+"/staging", cfg.ArduinoDataDir+"/user"))
		if err := os.WriteFile(cfg.ArduinoConfigFile, contents, 0o600); err != nil {
			return fmt.Errorf("write arduino config: %w", err)
		}
	}

	for _, args := range [][]string{{"core", "update-index"}, {"lib", "update-index"}} {
		if _, err := arduino.Command(ctx, cfg.ArduinoCLIPath, cfg.ArduinoConfigFile, args...); err != nil {
			logs.Error("Update indexes (%s): %v", strings.Join(args, " "), err)
		}
	}
	for _, core := range []string{"arduino:avr", "esp32:esp32"} {
		installed, err := arduino.IsCoreInstalled(ctx, cfg.ArduinoCLIPath, cfg.ArduinoConfigFile, core)
		if err != nil {
			logs.Error("Check core %s: %v", core, err)
		}
		if installed {
			continue
		}
		if _, err := arduino.InstallCore(ctx, cfg.ArduinoCLIPath, cfg.ArduinoConfigFile, core, nil); err != nil {
			logs.Error("Install core %s: %v", core, err)
			continue
		}
	}
	return nil
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && !allowedOrigin(origin) {
			http.Error(w, "Origin not allowed. Check COMPILER_ALLOWED_ORIGINS.", http.StatusForbidden)
			return
		}
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set(
			"Access-Control-Allow-Headers",
			"Content-Type, Authorization, X-Api-Key",
		)

		if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func allowedOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	if parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1" {
		return true
	}
	for _, value := range strings.Split(allowedOriginsList(), ",") {
		if strings.TrimRight(strings.TrimSpace(value), "/") == origin {
			return true
		}
	}
	return false
}

func allowedOriginsList() string {
	allowed := os.Getenv("COMPILER_ALLOWED_ORIGINS")
	if allowed == "" {
		allowed = "https://app.blockly.lumistem.org, https://demo.blockly.lumistem.org"
	}
	return allowed
}
