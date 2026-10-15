// Always trust Go's bundled root certificates: the receiver's own CA bundle
// is years out of date and rejects many modern HTTPS playlist hosts.
//go:debug x509usefallbackroots=1

package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	_ "golang.org/x/crypto/x509roots/fallback"

	"dreaminator/internal/app"
	"dreaminator/internal/server"
	"dreaminator/web"
)

var version = "dev"

func main() {
	listen := flag.String("listen", ":8088", "адрес веб-интерфейса")
	dataDir := flag.String("data", "/etc/dreaminator", "папка с настройками и кэшем")
	e2Dir := flag.String("e2dir", "/etc/enigma2", "папка с бакетами Enigma2")
	epgDir := flag.String("epgimport-dir", "/etc/epgimport", "папка настроек EPGImport")
	pluginDir := flag.String("plugin-dir", "/usr/lib/enigma2/python/Plugins/Extensions", "папка плагинов Enigma2")
	publicURL := flag.String("public-url", "", "адрес, по которому EPGImport забирает программу (по умолчанию http://127.0.0.1:<порт>)")
	webif := flag.String("webif", "http://127.0.0.1", "адрес OpenWebif по умолчанию (пусто — не обращаться к ресиверу)")
	logFile := flag.String("log", "", "файл журнала (по умолчанию stderr)")
	piconDir := flag.String("picon-dir", "", "папка пиконов (по умолчанию — та, которую использует Enigma2)")
	showVersion := flag.Bool("version", false, "показать версию")
	flag.Parse()

	if *showVersion {
		os.Stdout.WriteString(version + "\n")
		return
	}
	// The DM520 has little RAM to spare next to Enigma2.
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(96 << 20)
	}

	var out io.Writer = os.Stderr
	if *logFile != "" {
		if st, err := os.Stat(*logFile); err == nil && st.Size() > 1<<20 {
			os.Remove(*logFile)
		}
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		out = f
	}
	logger := log.New(out, "", log.LstdFlags)

	_, port, err := net.SplitHostPort(*listen)
	if err != nil {
		logger.Fatalf("неверный -listen: %v", err)
	}
	if *publicURL == "" {
		*publicURL = "http://127.0.0.1:" + port
	}

	defaults := app.DefaultSettings()
	defaults.WebifURL = *webif
	cfg := app.Config{
		DataDir:      *dataDir,
		E2Dir:        *e2Dir,
		EPGImportDir: *epgDir,
		PluginDir:    *pluginDir,
		PublicURL:    *publicURL,
		Port:         port,
		Version:      version,
	}
	if *piconDir != "" {
		cfg.PiconCandidates = []string{*piconDir}
	}
	a, err := app.New(cfg, defaults, logger)
	if err != nil {
		logger.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a.Start(ctx)

	srv := &http.Server{
		Addr:              *listen,
		Handler:           server.New(a, web.Static()),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	logger.Printf("dreaminator %s запущен на %s", version, *listen)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal(err)
	}
}
