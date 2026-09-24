// Package orders принимает заявки с сайта: проверяет поля, пишет их в
// JSONL-файл и, если настроен бот, отправляет уведомление в Telegram.
package orders

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Order — заявка, отправленная с сайта.
type Order struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Name      string    `json:"name"`
	Contact   string    `json:"contact"`
	Product   string    `json:"product"` // slug изделия или пусто для заявки на кастом
	Message   string    `json:"message"`
	// Files — пути приложенных файлов относительно каталога хранилища
	// (data/uploads/<ID>/<имя>), пусто, если ничего не приложено.
	Files []string `json:"files,omitempty"`
}

// Ограничения на длину полей: заявка приходит из открытой формы, поэтому
// размер проверяем до записи на диск.
const (
	maxName    = 100
	maxContact = 120
	maxMessage = 2000
	maxProduct = 80

	// MaxFiles и MaxFileSize — лимиты вложений формы заявки: не больше 5
	// файлов, каждый до 10 МБ.
	MaxFiles    = 5
	MaxFileSize = 10 << 20
)

// UploadedFile — файл, приложенный к заявке до сохранения на диск.
type UploadedFile struct {
	Name string
	Data []byte
}

var unsafeFileChars = regexp.MustCompile(`[^A-Za-zА-Яа-яЁё0-9._-]+`)

// sanitizeFileName убирает из имени файла всё, что не буква, цифра, точка,
// дефис или подчёркивание — так к нему нельзя добавить путь вроде "../".
func sanitizeFileName(name string) string {
	name = filepath.Base(name)
	name = unsafeFileChars.ReplaceAllString(name, "_")
	if name == "" || name == "." || name == ".." {
		name = "file"
	}
	if len(name) > 100 {
		name = name[len(name)-100:]
	}
	return name
}

// ValidationError описывает, какие поля формы заполнены неверно.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	return "orders: некорректные поля: " + strings.Join(keys, ", ")
}

// Validate проверяет заявку и нормализует пробелы в полях.
func (o *Order) Validate() error {
	o.Name = strings.TrimSpace(o.Name)
	o.Contact = strings.TrimSpace(o.Contact)
	o.Message = strings.TrimSpace(o.Message)
	o.Product = strings.TrimSpace(o.Product)

	fields := map[string]string{}
	switch {
	case o.Name == "":
		fields["name"] = "Укажите, как к вам обращаться"
	case utf8.RuneCountInString(o.Name) > maxName:
		fields["name"] = fmt.Sprintf("Не длиннее %d символов", maxName)
	}
	switch {
	case o.Contact == "":
		fields["contact"] = "Оставьте Telegram, телефон или почту"
	case utf8.RuneCountInString(o.Contact) > maxContact:
		fields["contact"] = fmt.Sprintf("Не длиннее %d символов", maxContact)
	}
	if utf8.RuneCountInString(o.Message) > maxMessage {
		fields["message"] = fmt.Sprintf("Не длиннее %d символов", maxMessage)
	}
	if utf8.RuneCountInString(o.Product) > maxProduct {
		fields["product"] = "Неизвестное изделие"
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

// Notifier отправляет уведомление о заявке во внешний канал вместе с
// приложенными файлами (nil или пустой список — заявка без вложений).
type Notifier interface {
	Notify(o Order, files []UploadedFile) error
}

// ValidateFiles проверяет вложения формы: не больше MaxFiles штук и не
// больше MaxFileSize байт каждый.
func ValidateFiles(headers []*multipart.FileHeader) error {
	if len(headers) > MaxFiles {
		return &ValidationError{Fields: map[string]string{
			"files": fmt.Sprintf("Не больше %d файлов", MaxFiles),
		}}
	}
	for _, h := range headers {
		if h.Size > MaxFileSize {
			return &ValidationError{Fields: map[string]string{
				"files": fmt.Sprintf("«%s» больше 10 МБ", h.Filename),
			}}
		}
	}
	return nil
}

// ReadFiles копирует содержимое вложений формы в память — этого достаточно
// для заявки: файлов немного и каждый ограничен MaxFileSize.
func ReadFiles(headers []*multipart.FileHeader) ([]UploadedFile, error) {
	out := make([]UploadedFile, 0, len(headers))
	for _, h := range headers {
		f, err := h.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(f, MaxFileSize+1))
		f.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, UploadedFile{Name: h.Filename, Data: data})
	}
	return out, nil
}

// Store принимает и хранит заявки. Нулевое значение непригодно — используйте NewStore.
type Store struct {
	mu       sync.Mutex
	path     string
	notifier Notifier
	log      *slog.Logger
	seq      int
	now      func() time.Time
}

// NewStore создаёт хранилище заявок. path — файл в формате JSON Lines;
// пустой path отключает запись на диск. notifier и log могут быть nil.
func NewStore(path string, n Notifier, log *slog.Logger) *Store {
	if log == nil {
		log = slog.Default()
	}
	return &Store{path: path, notifier: n, log: log, now: time.Now}
}

// Submit проверяет заявку и сохраняет её вместе с вложениями, после чего
// отправляет уведомление. Недоставленное уведомление не отменяет приём:
// заявка уже на диске, поэтому такая ошибка только пишется в лог.
func (s *Store) Submit(o Order, files []UploadedFile) (Order, error) {
	if err := o.Validate(); err != nil {
		return o, err
	}

	s.mu.Lock()
	s.seq++
	now := s.now().UTC()
	o.CreatedAt = now
	o.ID = fmt.Sprintf("KP-%s-%03d", now.Format("20060102-150405"), s.seq)
	path := s.path
	s.mu.Unlock()

	if path != "" && len(files) > 0 {
		saved, err := saveFiles(path, o.ID, files)
		if err != nil {
			return o, fmt.Errorf("сохранение вложений: %w", err)
		}
		o.Files = saved
	}

	if path != "" {
		if err := s.append(path, o); err != nil {
			return o, fmt.Errorf("сохранение заявки: %w", err)
		}
	}
	if s.notifier != nil {
		if err := s.notifier.Notify(o, files); err != nil {
			s.log.Error("заявка принята, но уведомление не доставлено", "id", o.ID, "err", err)
		}
	}
	return o, nil
}

// saveFiles пишет вложения на диск рядом с файлом заявок, в подкаталог
// uploads/<ID>, и возвращает их пути относительно каталога хранилища.
func saveFiles(ordersPath, orderID string, files []UploadedFile) ([]string, error) {
	dir := filepath.Join(filepath.Dir(ordersPath), "uploads", orderID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	saved := make([]string, 0, len(files))
	for _, f := range files {
		name := sanitizeFileName(f.Name)
		if err := os.WriteFile(filepath.Join(dir, name), f.Data, 0o644); err != nil {
			return saved, err
		}
		saved = append(saved, filepath.ToSlash(filepath.Join("uploads", orderID, name)))
	}
	return saved, nil
}

func (s *Store) append(path string, o Order) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	line, err := json.Marshal(o)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// TelegramNotifier шлёт заявку в чат через Bot API. Создаётся только когда
// заданы переменные окружения TELEGRAM_BOT_TOKEN и TELEGRAM_CHAT_ID.
type TelegramNotifier struct {
	Token  string
	ChatID string
	Client *http.Client
	// APIBase позволяет подменить адрес Bot API в тестах.
	APIBase string
}

// NewTelegramNotifier возвращает nil, если бот не настроен.
func NewTelegramNotifier(token, chatID string) *TelegramNotifier {
	if token == "" || chatID == "" {
		return nil
	}
	return &TelegramNotifier{
		Token:   token,
		ChatID:  chatID,
		Client:  &http.Client{Timeout: 10 * time.Second},
		APIBase: "https://api.telegram.org",
	}
}

// Notify отправляет сообщение о заявке в Telegram, а затем — каждое
// вложение отдельным документом. Ошибка при отправке одного файла не
// прерывает остальные: до получателя должно дойти как можно больше.
func (t *TelegramNotifier) Notify(o Order, files []UploadedFile) error {
	var b strings.Builder
	b.WriteString("Заявка с сайта " + o.ID + "\n")
	b.WriteString("Имя: " + o.Name + "\n")
	b.WriteString("Контакт: " + o.Contact + "\n")
	if o.Product != "" {
		b.WriteString("Изделие: " + o.Product + "\n")
	}
	if o.Message != "" {
		b.WriteString("Сообщение: " + o.Message)
	}
	if len(files) > 0 {
		b.WriteString(fmt.Sprintf("\nВложений: %d", len(files)))
	}

	form := url.Values{}
	form.Set("chat_id", t.ChatID)
	form.Set("text", b.String())
	form.Set("disable_web_page_preview", "true")

	client := t.client()
	base := t.apiBase()
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", base, t.Token)
	resp, err := client.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var body bytes.Buffer
		_, _ = body.ReadFrom(resp.Body)
		return fmt.Errorf("telegram api: %s: %s", resp.Status, strings.TrimSpace(body.String()))
	}

	var sendErr error
	for _, f := range files {
		if err := t.sendDocument(client, base, f); err != nil {
			sendErr = err
		}
	}
	return sendErr
}

func (t *TelegramNotifier) client() *http.Client {
	if t.Client != nil {
		return t.Client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (t *TelegramNotifier) apiBase() string {
	if t.APIBase != "" {
		return strings.TrimRight(t.APIBase, "/")
	}
	return "https://api.telegram.org"
}

// sendDocument отправляет одно вложение через sendDocument Bot API.
func (t *TelegramNotifier) sendDocument(client *http.Client, base string, f UploadedFile) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("chat_id", t.ChatID); err != nil {
		return err
	}
	part, err := w.CreateFormFile("document", sanitizeFileName(f.Name))
	if err != nil {
		return err
	}
	if _, err := part.Write(f.Data); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendDocument", base, t.Token)
	resp, err := client.Post(endpoint, w.FormDataContentType(), &body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var respBody bytes.Buffer
		_, _ = respBody.ReadFrom(resp.Body)
		return fmt.Errorf("telegram api sendDocument: %s: %s", resp.Status, strings.TrimSpace(respBody.String()))
	}
	return nil
}
