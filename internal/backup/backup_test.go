package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Подпись сверена с эталонной реализацией (botocore S3SigV4Auth) на тех же
// входных данных: кириллица и пробел в ключе, метаданные, свой регион.
func TestSignMatchesReference(t *testing.T) {
	c := &s3Client{
		endpoint: "https://s3.ru-6.storage.selcloud.ru", region: "ru-6", bucket: "my-bucket",
		accessKey: "AKID", secretKey: "SECRET/KEY",
		now: func() time.Time { return time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC) },
	}
	u, err := c.objectURL("backups/заявки data.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPut, u.String(), nil)
	req.Header.Set("content-type", "application/gzip")
	req.Header.Set("x-amz-meta-orders", "7")
	c.sign(req, sha256Hex([]byte("hello")))

	want := "AWS4-HMAC-SHA256 Credential=AKID/20261117/ru-6/s3/aws4_request, " +
		"SignedHeaders=content-type;host;x-amz-content-sha256;x-amz-date;x-amz-meta-orders, " +
		"Signature=54733321a56e62ee59fe53a7685fb7062888ae86b160dfab3805e2f0644825ec"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("подпись не совпала с эталоном:\n got %s\nwant %s", got, want)
	}
}

// fakeS3 — S3 в памяти: хранит объекты и их x-amz-meta-* заголовки.
type fakeS3 struct {
	mu   sync.Mutex
	objs map[string][]byte
	meta map[string]http.Header
	puts int
	fail bool
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		http.Error(w, "boom", http.StatusInternalServerError)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
		http.Error(w, "no auth", http.StatusForbidden)
		return
	}
	key := r.URL.Path
	switch r.Method {
	case http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		if sum := sha256Hex(body); sum != r.Header.Get("x-amz-content-sha256") {
			http.Error(w, "bad sha", http.StatusBadRequest)
			return
		}
		f.objs[key] = body
		m := http.Header{}
		for k, v := range r.Header {
			if strings.HasPrefix(strings.ToLower(k), "x-amz-meta-") {
				m[k] = v
			}
		}
		f.meta[key] = m
		f.puts++
	case http.MethodHead, http.MethodGet:
		body, ok := f.objs[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		for k, v := range f.meta[key] {
			w.Header()[k] = v
		}
		if r.Method == http.MethodGet {
			w.Write(body)
		}
	}
}

func newTest(t *testing.T) (*fakeS3, func(dir string) *Service) {
	t.Helper()
	fake := &fakeS3{objs: map[string][]byte{}, meta: map[string]http.Header{}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, func(dir string) *Service {
		s := New(Config{
			Endpoint: srv.URL, Region: "ru-1", Bucket: "b", AccessKey: "k", SecretKey: "s",
			Key: "backups/data.tar.gz", Interval: time.Hour, RestoreOnStart: "always",
		}, filepath.Join(dir, "orders.jsonl"), slog.New(slog.NewTextHandler(io.Discard, nil)))
		s.retry = time.Millisecond
		return s
	}
}

func write(t *testing.T, p, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestBackupAndRestoreRoundTrip(t *testing.T) {
	fake, mk := newTest(t)
	ctx := context.Background()

	src := t.TempDir()
	write(t, filepath.Join(src, "orders.jsonl"), "{\"id\":\"1\"}\n{\"id\":\"2\"}\n{\"id\":\"3\"") // третья недописана
	write(t, filepath.Join(src, "uploads", "KP-1", "фото.jpg"), "JPEG")
	if err := mk(src).Backup(ctx); err != nil {
		t.Fatal(err)
	}
	if got := fake.meta["/b/backups/data.tar.gz"].Get("x-amz-meta-orders"); got != "2" {
		t.Fatalf("метаданные orders = %q, want 2", got)
	}

	// Новый сервер с пустым каталогом: старт подтягивает данные из S3.
	dst := t.TempDir()
	s := mk(dst)
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dst, "orders.jsonl")); got != "{\"id\":\"1\"}\n{\"id\":\"2\"}\n" {
		t.Fatalf("заявки после восстановления: %q", got)
	}
	if got := read(t, filepath.Join(dst, "uploads", "KP-1", "фото.jpg")); got != "JPEG" {
		t.Fatalf("вложение после восстановления: %q", got)
	}

	// Сразу после восстановления данные совпадают с облаком — выгрузки нет.
	puts := fake.puts
	if err := s.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	if fake.puts != puts {
		t.Fatal("неизменённые данные выгружены повторно")
	}

	// Новая заявка — бэкап обновляется.
	write(t, filepath.Join(dst, "orders.jsonl"), "{\"id\":\"1\"}\n{\"id\":\"2\"}\n{\"id\":\"3\"}\n")
	if err := s.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	if fake.puts != puts+1 || fake.meta["/b/backups/data.tar.gz"].Get("x-amz-meta-orders") != "3" {
		t.Fatal("изменённые данные не выгружены")
	}
}

func TestEmptyDataNeverOverwritesBackup(t *testing.T) {
	fake, mk := newTest(t)
	ctx := context.Background()
	src := t.TempDir()
	write(t, filepath.Join(src, "orders.jsonl"), "{}\n")
	if err := mk(src).Backup(ctx); err != nil {
		t.Fatal(err)
	}
	before := fake.objs["/b/backups/data.tar.gz"]

	empty := mk(t.TempDir())
	empty.cfg.RestoreOnStart = "off"
	if err := empty.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := empty.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fake.objs["/b/backups/data.tar.gz"], before) {
		t.Fatal("пустые данные затёрли бэкап")
	}
}

func TestStartKeepsNewerLocalData(t *testing.T) {
	_, mk := newTest(t)
	ctx := context.Background()
	old := t.TempDir()
	write(t, filepath.Join(old, "orders.jsonl"), "{}\n")
	if err := mk(old).Backup(ctx); err != nil {
		t.Fatal(err)
	}
	cur := t.TempDir()
	local := "{}\n{}\n"
	write(t, filepath.Join(cur, "orders.jsonl"), local)
	if err := mk(cur).Start(ctx); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(cur, "orders.jsonl")); got != local {
		t.Fatalf("старый бэкап затёр более свежие локальные заявки: %q", got)
	}
}

func TestRestoreModeEmpty(t *testing.T) {
	_, mk := newTest(t)
	ctx := context.Background()
	src := t.TempDir()
	write(t, filepath.Join(src, "orders.jsonl"), "{}\n{}\n")
	if err := mk(src).Backup(ctx); err != nil {
		t.Fatal(err)
	}
	cur := t.TempDir()
	write(t, filepath.Join(cur, "orders.jsonl"), "{\"local\":1}\n")
	s := mk(cur)
	s.cfg.RestoreOnStart = "empty"
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(cur, "orders.jsonl")); got != "{\"local\":1}\n" {
		t.Fatalf("режим empty восстановил непустые данные: %q", got)
	}
}

func TestFailedRestoreDisablesUploads(t *testing.T) {
	fake, mk := newTest(t)
	fake.fail = true
	dir := t.TempDir()
	write(t, filepath.Join(dir, "orders.jsonl"), "{}\n")
	s := mk(dir)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.fail = false
	fake.mu.Unlock()
	s.Final(time.Second)
	if fake.puts != 0 {
		t.Fatal("после неудачного восстановления бэкап всё равно выгружен")
	}
}

func TestUnpackRejectsPathTraversal(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range []string{"../evil.txt", "uploads/../../evil2.txt", "/etc/evil3", "orders.jsonl", "uploads/x/a.txt"} {
		tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o644, Size: 2})
		tw.Write([]byte("ok"))
	}
	tw.Close()
	gz.Close()

	root := t.TempDir()
	dir := filepath.Join(root, "data")
	if err := unpack(&buf, filepath.Join(dir, "orders.jsonl")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(root, "evil.txt"), filepath.Join(root, "evil2.txt"), filepath.Join(dir, "evil3")} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("архив записал файл за пределы каталога данных: %s", p)
		}
	}
	if read(t, filepath.Join(dir, "uploads", "x", "a.txt")) != "ok" || read(t, filepath.Join(dir, "orders.jsonl")) != "ok" {
		t.Fatal("допустимые файлы не восстановлены")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".restore-") {
			t.Fatal("временный каталог восстановления не удалён")
		}
	}
}

func TestDisabledWithoutCredentials(t *testing.T) {
	var s *Service = New(Config{Bucket: "b"}, "data/orders.jsonl", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if s != nil {
		t.Fatal("без ключей бэкап должен быть выключен")
	}
	// методы nil-сервиса ничего не делают и не паникуют
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.Run(context.Background())
	s.Final(time.Second)
}
