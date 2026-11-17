// Package backup хранит копию данных сайта (файл заявок и вложения) в S3:
// один объект, перезаписывается каждые N минут.
//
// Жизненный цикл (тот же, что у бэкапа базы в Web-site_mother):
//
//	старт      — данные восстанавливаются из S3 (BACKUP_RESTORE_ON_START)
//	             до того, как сайт начнёт принимать запросы;
//	работа     — бэкап каждые BACKUP_INTERVAL_MINUTES;
//	остановка  — на SIGTERM/SIGINT (docker stop, restart, перезагрузка
//	             сервера) сначала снимается свежий бэкап, потом выход.
//
// Пустые данные (нет заявок) никогда не перезаписывают бэкап, где они были:
// иначе обнулившийся каталог через 30 минут затёр бы и единственную копию.
package backup

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config — настройки бэкапа. Переменные окружения те же, что в Web-site_mother.
type Config struct {
	Endpoint  string // S3_ENDPOINT
	Region    string // S3_REGION
	Bucket    string // BACKUP_S3_BUCKET, иначе S3_BUCKET
	AccessKey string // S3_ACCESS_KEY
	SecretKey string // S3_SECRET_KEY
	Key       string // BACKUP_S3_KEY
	Interval  time.Duration
	// RestoreOnStart: always — при каждом старте; empty — только если
	// локально заявок нет; off — никогда.
	RestoreOnStart string
}

// ConfigFromEnv читает настройки из окружения.
func ConfigFromEnv(getenv func(string) string) Config {
	or := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}
	minutes, err := strconv.Atoi(or("BACKUP_INTERVAL_MINUTES", "30"))
	if err != nil || minutes < 1 {
		minutes = 30
	}
	return Config{
		Endpoint:       or("S3_ENDPOINT", "https://s3.storage.selcloud.ru"),
		Region:         or("S3_REGION", "ru-1"),
		Bucket:         or("BACKUP_S3_BUCKET", or("S3_BUCKET", "")),
		AccessKey:      or("S3_ACCESS_KEY", ""),
		SecretKey:      or("S3_SECRET_KEY", ""),
		Key:            or("BACKUP_S3_KEY", "backups/kinoperiferia-data.tar.gz"),
		Interval:       time.Duration(minutes) * time.Minute,
		RestoreOnStart: strings.ToLower(or("BACKUP_RESTORE_ON_START", "always")),
	}
}

// Enabled — заданы бакет и ключи доступа.
func (c Config) Enabled() bool {
	return c.Bucket != "" && c.AccessKey != "" && c.SecretKey != ""
}

// Service снимает и восстанавливает бэкапы. Все методы безопасно вызывать
// у nil: так выглядит сайт без настроенного S3.
type Service struct {
	cfg        Config
	ordersPath string
	s3         *s3Client
	log        *slog.Logger

	mu      sync.Mutex // бэкапы не пересекаются: периодический и финальный
	uploads bool       // false — восстановление не удалось, выгрузка запрещена
	retry   time.Duration
}

// New возвращает nil, если S3 не настроен или файл заявок не задан.
func New(cfg Config, ordersPath string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	if !cfg.Enabled() || ordersPath == "" {
		log.Warn("бэкап выключен: не заданы S3_BUCKET, S3_ACCESS_KEY, S3_SECRET_KEY")
		return nil
	}
	return &Service{
		cfg:        cfg,
		ordersPath: ordersPath,
		log:        log.With("component", "backup"),
		s3: &s3Client{
			endpoint:  cfg.Endpoint,
			region:    cfg.Region,
			bucket:    cfg.Bucket,
			accessKey: cfg.AccessKey,
			secretKey: cfg.SecretKey,
			http:      &http.Client{Timeout: 10 * time.Minute},
		},
		uploads: true,
		retry:   5 * time.Second,
	}
}

// remoteOrders — сколько заявок в бэкапе; -1 — бэкапа нет.
func (s *Service) remoteOrders(ctx context.Context) (int, map[string]string, error) {
	meta, err := s.s3.head(ctx, s.cfg.Key)
	if errors.Is(err, errNotFound) {
		return -1, nil, nil
	}
	if err != nil {
		return 0, nil, err
	}
	n, _ := strconv.Atoi(meta["orders"])
	return n, meta, nil
}

// Backup снимает бэкап сейчас. Пропускает выгрузку, если данные не
// изменились или если заявок нет, а в облаке они есть.
func (s *Service) Backup(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.uploads {
		return nil
	}

	local, err := countOrders(s.ordersPath)
	if err != nil {
		return err
	}
	prev, meta, err := s.remoteOrders(ctx)
	if err != nil {
		return err
	}
	if local == 0 && prev > 0 {
		s.log.Warn("локально 0 заявок, а в бэкапе есть — бэкап НЕ перезаписан", "в бэкапе", prev)
		return nil
	}

	f, size, sum, err := pack(s.ordersPath)
	if err != nil {
		return err
	}
	defer func() { f.Close(); os.Remove(f.Name()) }()

	if meta != nil && meta["sha256"] == sum {
		s.log.Debug("данные не изменились — выгрузка не нужна")
		return nil
	}
	err = s.s3.put(ctx, s.cfg.Key, f, size, sum, map[string]string{
		"orders":  strconv.Itoa(local),
		"created": strconv.FormatInt(time.Now().Unix(), 10),
		"sha256":  sum,
	})
	if err != nil {
		return err
	}
	s.log.Info("бэкап загружен", "s3", "s3://"+s.cfg.Bucket+"/"+s.cfg.Key, "заявок", local, "КБ", size/1024)
	return nil
}

// Restore принудительно заменяет локальные данные бэкапом из S3.
func (s *Service) Restore(ctx context.Context) error {
	if s == nil {
		return nil
	}
	tmp, err := os.CreateTemp("", "kinoperiferia-restore-*.tar.gz")
	if err != nil {
		return err
	}
	defer func() { tmp.Close(); os.Remove(tmp.Name()) }()

	if err := s.s3.get(ctx, s.cfg.Key, tmp); err != nil {
		return err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := unpack(tmp, s.ordersPath); err != nil {
		return err
	}
	n, _ := countOrders(s.ordersPath)
	s.log.Info("данные восстановлены из S3", "s3", "s3://"+s.cfg.Bucket+"/"+s.cfg.Key, "заявок", n)
	return nil
}

// restoreOnStart подтягивает данные из S3 до запуска сайта. Не
// восстанавливает, если в бэкапе заявок меньше, чем локально: так бывает
// после падения сервера, когда последний бэкап старше данных, —
// восстановление стёрло бы заявки, пришедшие после него.
func (s *Service) restoreOnStart(ctx context.Context) error {
	prev, _, err := s.remoteOrders(ctx)
	if err != nil {
		return err
	}
	if prev < 0 {
		s.log.Info("в S3 нет бэкапа — запускаемся с текущими данными")
		return nil
	}
	local, err := countOrders(s.ordersPath)
	if err != nil {
		return err
	}
	mode := s.cfg.RestoreOnStart
	if mode == "off" || (mode == "empty" && local > 0) {
		s.log.Info("восстановление при старте выключено", "режим", mode, "заявок локально", local)
		return nil
	}
	if prev < local {
		s.log.Warn("в бэкапе заявок меньше, чем локально — НЕ восстанавливаю, чтобы не потерять новые",
			"локально", local, "в бэкапе", prev)
		return nil
	}
	s.log.Info("восстанавливаю данные из S3", "в бэкапе", prev, "локально", local)
	return s.Restore(ctx)
}

// Start восстанавливает данные перед запуском сайта (3 попытки). Если все
// попытки неудачны, сайт всё равно запускается, но выгрузка в S3 отключается
// до перезапуска: устаревшие данные затёрли бы более свежую облачную копию.
// Ошибку возвращает, только если пришёл сигнал остановки (ctx отменён) —
// тогда бэкап «на выходе» снимать нельзя, он залил бы старые данные.
func (s *Service) Start(ctx context.Context) error {
	if s == nil {
		return nil
	}
	for attempt := 1; attempt <= 3; attempt++ {
		err := s.restoreOnStart(ctx)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			s.mu.Lock()
			s.uploads = false
			s.mu.Unlock()
			return ctx.Err()
		}
		s.log.Error("восстановление при старте не удалось", "попытка", attempt, "из", 3, "err", err)
		select {
		case <-ctx.Done():
		case <-time.After(s.retry):
		}
	}
	s.mu.Lock()
	s.uploads = false
	s.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.log.Error("данные НЕ восстановлены — сайт запускается, но бэкапы в S3 до " +
		"перезапуска отключены, чтобы не затереть облачную копию")
	return nil
}

// Run снимает бэкап сразу и затем каждые Interval, пока ctx не отменён.
func (s *Service) Run(ctx context.Context) {
	if s == nil {
		return
	}
	for {
		opCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		if err := s.Backup(opCtx); err != nil && ctx.Err() == nil {
			// сеть/S3 моргнули — попробуем в следующий раз
			s.log.Error("бэкап не удался", "err", err)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.cfg.Interval):
		}
	}
}

// Final снимает финальный бэкап при остановке сайта.
func (s *Service) Final(timeout time.Duration) {
	if s == nil {
		return
	}
	s.mu.Lock()
	ok := s.uploads
	s.mu.Unlock()
	if !ok {
		s.log.Warn("остановка: финальный бэкап пропущен — данные не были восстановлены")
		return
	}
	s.log.Info("остановка: снимаю финальный бэкап")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.Backup(ctx); err != nil {
		s.log.Error("финальный бэкап не удался — в S3 остался предыдущий", "err", err)
	}
}
