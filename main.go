// Команда kinoperiferia поднимает сайт «Кинопериферии»: витрину каталога,
// страницы изделий, JSON API и приём заявок.
package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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
		Handler:           logRequests(log, app),
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
