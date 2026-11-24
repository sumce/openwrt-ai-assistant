// WRT AI — an AI assistant that runs on OpenWRT and operates the router
// through tools (status, logs, diagnostics, UCI configuration).
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"runtime"

	"github.com/sumce/openwrt-ai-assistant/internal/agent"
	"github.com/sumce/openwrt-ai-assistant/internal/config"
	"github.com/sumce/openwrt-ai-assistant/internal/runstate"
	"github.com/sumce/openwrt-ai-assistant/internal/safety"
	"github.com/sumce/openwrt-ai-assistant/internal/server"
	"github.com/sumce/openwrt-ai-assistant/internal/store"
	"github.com/sumce/openwrt-ai-assistant/internal/tools"
)

func main() {
	port := flag.Int("port", 0, "override listen port (default: from config)")
	listen := flag.String("listen", "", "override listen address (default: from config)")
	dataDir := flag.String("data", "", "data directory (default /etc/wrtai; dev mode: ./devdata)")
	dev := flag.Bool("dev", runtime.GOOS != "linux", "dev mode: JSON settings + mocked router tools")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("wrtai", server.Version)
		return
	}

	dir := *dataDir
	if dir == "" {
		if *dev {
			dir = "devdata"
		} else {
			dir = "/etc/wrtai"
		}
	}

	var settings config.Store
	var platform tools.Platform
	if *dev {
		settings = config.NewJSONStore(filepath.Join(dir, "settings.json"))
		platform = tools.NewMock()
		log.Printf("dev mode: mocked router tools + JSON settings")
	} else {
		settings = config.NewUCIStore("/etc/config/wrtai")
		platform = tools.NewReal()
	}

	registry := tools.NewRegistry(platform)
	backups := safety.NewBackups(filepath.Join(dir, "backups"), platform)

	runStore, err := runstate.New(filepath.Join(dir, "runs"))
	if err != nil {
		log.Fatalf("init run state dir: %v", err)
	}
	convs, err := store.New(filepath.Join(dir, "conversations"))
	if err != nil {
		log.Fatalf("init conversations dir: %v", err)
	}
	auth, err := server.NewAuth(dir)
	if err != nil {
		log.Fatalf("init auth: %v", err)
	}

	manager := agent.NewManager(settings, registry, backups, runStore, convs)
	manager.BootstrapRestore()

	st, err := settings.Load()
	if err != nil {
		log.Fatalf("load settings: %v", err)
	}

	listenAddr := st.Listen
	if *listen != "" {
		listenAddr = *listen
	}
	if listenAddr == "" {
		listenAddr = "0.0.0.0"
	}
	listenPort := st.Port
	if *port != 0 {
		listenPort = *port
	}
	if listenPort <= 0 {
		listenPort = 8765
	}
	addr := fmt.Sprintf("%s:%d", listenAddr, listenPort)

	// Dev convenience: seed the scripted fake LLM so the full flow works
	// without any external API key.
	if *dev && len(st.Providers) == 0 {
		st.Providers = append(st.Providers, config.Provider{
			ID:      "mock",
			Name:    "内置模拟模型",
			BaseURL: fmt.Sprintf("http://127.0.0.1:%d/fake-llm/v1", listenPort),
			APIKey:  "-",
			Model:   "wrtai-mock",
		})
		st.DefaultProvider = "mock"
		if err := settings.Save(st); err != nil {
			log.Printf("seed fake provider: %v", err)
		}
	}

	handler := server.New(server.Config{
		Auth:     auth,
		Settings: settings,
		Manager:  manager,
		Backups:  backups,
		Convs:    convs,
		Dev:      *dev,
	})

	log.Printf("WRT AI %s → http://%s  (data=%s)", server.Version, addr, dir)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
