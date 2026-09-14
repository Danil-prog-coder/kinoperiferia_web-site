// Package orders принимает заявки с сайта: проверяет поля, пишет их в
// JSONL-файл и, если настроен бот, отправляет уведомление в Telegram.
package orders

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
}

// Ограничения на длину полей: заявка приходит из открытой формы, поэтому
// размер проверяем до записи на диск.
const (
	maxName    = 100
	maxContact = 120
	maxMessage = 2000
	maxProduct = 80
)

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

// Notifier отправляет уведомление о заявке во внешний канал.
type Notifier interface {
	Notify(o Order) error
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

// Submit проверяет заявку и сохраняет её, после чего отправляет уведомление.
// Недоставленное уведомление не отменяет приём: заявка уже на диске, поэтому
// такая ошибка только пишется в лог.
func (s *Store) Submit(o Order) (Order, error) {
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

	if path != "" {
		if err := s.append(path, o); err != nil {
			return o, fmt.Errorf("сохранение заявки: %w", err)
		}
	}
	if s.notifier != nil {
		if err := s.notifier.Notify(o); err != nil {
			s.log.Error("заявка принята, но уведомление не доставлено", "id", o.ID, "err", err)
		}
	}
	return o, nil
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

// Notify отправляет сообщение о заявке в Telegram.
func (t *TelegramNotifier) Notify(o Order) error {
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

	form := url.Values{}
	form.Set("chat_id", t.ChatID)
	form.Set("text", b.String())
	form.Set("disable_web_page_preview", "true")

	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	base := t.APIBase
	if base == "" {
		base = "https://api.telegram.org"
	}
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", strings.TrimRight(base, "/"), t.Token)
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
	return nil
}
