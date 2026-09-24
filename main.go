// Команда kinoperiferia поднимает сайт «Кинопериферии»: витрину каталога,
// страницы изделий, JSON API и приём заявок.
package main

import (
	"compress/gzip"
	"context"
	"embed"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/danil-prog-coder/kinoperiferia_web-site/internal/orders"
	"github.com/danil-prog-coder/kinoperiferia_web-site/internal/server"
)

//go:embed web/templates/*.gohtml
var templatesFS embed.FS

//go:embed web/static
var staticFS embed.FS

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("сервер остановлен с ошибкой", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	templates, err := fs.Sub(templatesFS, "web/templates")
	if err != nil {
		return err
	}
	static, err := fs.Sub(staticFS, "web/static")
	if err != nil {
		return err
	}

	notifier := orders.NewTelegramNotifier(
		os.Getenv("TELEGRAM_BOT_TOKEN"),
		os.Getenv("TELEGRAM_CHAT_ID"),
	)
	if notifier == nil {
		log.Info("уведомления в Telegram выключены: не заданы TELEGRAM_BOT_TOKEN и TELEGRAM_CHAT_ID")
	}

	// orders.Store принимает интерфейс Notifier, поэтому nil-указатель нужно
	// превратить в nil-интерфейс — иначе проверка n != nil внутри пройдёт.
	var n orders.Notifier
	if notifier != nil {
		n = notifier
	}

	store := orders.NewStore(env("ORDERS_FILE", "data/orders.jsonl"), n, log)

	app, err := server.New(templates, static, server.Options{
		BaseURL: env("BASE_URL", ""),
		Orders:  store,
		Logger:  log,
	})
	if err != nil {
		return err
	}

	addr := ":" + env("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequests(log, gzipMiddleware(log, app)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		log.Info("сайт запущен", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
			return
		}
		errc <- nil
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		log.Info("получен сигнал остановки, завершаем соединения")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// statusRecorder запоминает код ответа, чтобы попасть в лог запроса.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// gzipMiddleware сжимает текстовые ответы (HTML/CSS/JS/JSON/XML), когда
// клиент это поддерживает. Статика уже версионирована и закэширована
// (assets.go), но не сжата — этот слой закрывает и её, и HTML-страницы.
func gzipMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	pool := sync.Pool{New: func() any { return gzip.NewWriter(io.Discard) }}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || r.Header.Get("Range") != "" {
			next.ServeHTTP(w, r)
			return
		}

		gz := pool.Get().(*gzip.Writer)
		gz.Reset(w)
		defer func() {
			if err := gz.Close(); err != nil {
				log.Debug("не удалось закрыть gzip-writer", "err", err)
			}
			pool.Put(gz)
		}()

		gzw := &gzipResponseWriter{ResponseWriter: w, gz: gz}
		next.ServeHTTP(gzw, r)
	})
}

// gzipResponseWriter включает сжатие только для текстовых типов и только
// после того, как обработчик определил Content-Type — иначе можно сжать
// уже закодированный ответ (например, изображение) или ответ без тела.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
	compress    bool
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		if isCompressible(w.Header().Get("Content-Type")) {
			w.compress = true
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Add("Vary", "Accept-Encoding")
			w.Header().Del("Content-Length")
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.compress {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func isCompressible(contentType string) bool {
	for _, prefix := range []string{"text/", "application/json", "application/xml", "application/javascript", "image/svg+xml"} {
		if strings.HasPrefix(contentType, prefix) {
			return true
		}
	}
	return false
}

func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Info("запрос",
			"method", r.Method,
			"path", r.URL.Path,
			"status", strconv.Itoa(rec.status),
			"ms", time.Since(start).Milliseconds(),
		)
	})
}
