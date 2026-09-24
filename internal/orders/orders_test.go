package orders

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRequiresNameAndContact(t *testing.T) {
	o := Order{Name: "  ", Contact: ""}
	err := o.Validate()

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("ожидалась ошибка валидации, получено %v", err)
	}
	if _, ok := verr.Fields["name"]; !ok {
		t.Error("нет ошибки по полю name")
	}
	if _, ok := verr.Fields["contact"]; !ok {
		t.Error("нет ошибки по полю contact")
	}
}

func TestValidateTrimsAndPassesGoodOrder(t *testing.T) {
	o := Order{Name: "  Дмитрий ", Contact: " @suvorov_dmitry ", Message: "  нужен чехол "}
	if err := o.Validate(); err != nil {
		t.Fatalf("корректная заявка не прошла: %v", err)
	}
	if o.Name != "Дмитрий" || o.Contact != "@suvorov_dmitry" || o.Message != "нужен чехол" {
		t.Errorf("пробелы не обрезаны: %+v", o)
	}
}

func TestValidateRejectsOverlongFields(t *testing.T) {
	o := Order{Name: strings.Repeat("я", maxName+1), Contact: "ок"}
	var verr *ValidationError
	if !errors.As(o.Validate(), &verr) {
		t.Fatal("слишком длинное имя прошло валидацию")
	}
	if _, ok := verr.Fields["name"]; !ok {
		t.Error("нет ошибки по длине имени")
	}
}

func TestSubmitWritesJSONLAndAssignsID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "orders.jsonl")
	store := NewStore(path, nil, nil)

	first, err := store.Submit(Order{Name: "А", Contact: "@a"}, nil)
	if err != nil {
		t.Fatalf("заявка не принята: %v", err)
	}
	second, _ := store.Submit(Order{Name: "Б", Contact: "@b"}, nil)

	if first.ID == "" || first.ID == second.ID {
		t.Errorf("идентификаторы не уникальны: %q и %q", first.ID, second.ID)
	}
	if first.CreatedAt.IsZero() {
		t.Error("не проставлено время заявки")
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("файл заявок не создан: %v", readErr)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("в файле %d строк, ожидалось 2", len(lines))
	}
	var got Order
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("строка не разбирается как JSON: %v", err)
	}
	if got.Name != "А" || got.ID != first.ID {
		t.Errorf("записана не та заявка: %+v", got)
	}
}

func TestSubmitRejectsInvalidWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orders.jsonl")
	store := NewStore(path, nil, nil)

	if _, err := store.Submit(Order{Name: "", Contact: ""}, nil); err == nil {
		t.Fatal("пустая заявка принята")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("файл заявок создан для отклонённой заявки")
	}
}

type failingNotifier struct{ calls int }

func (f *failingNotifier) Notify(Order, []UploadedFile) error {
	f.calls++
	return errors.New("канал недоступен")
}

func TestSubmitKeepsOrderWhenNotifyFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orders.jsonl")
	n := &failingNotifier{}
	store := NewStore(path, n, discardLogger())

	order, err := store.Submit(Order{Name: "А", Contact: "@a"}, nil)
	if err != nil {
		t.Fatalf("недоставленное уведомление отменило заявку: %v", err)
	}
	if order.ID == "" {
		t.Error("заявка без идентификатора")
	}
	if n.calls != 1 {
		t.Errorf("notifier вызван %d раз, ожидался 1", n.calls)
	}
	if data, readErr := os.ReadFile(path); readErr != nil || len(data) == 0 {
		t.Error("заявка не сохранена, хотя уведомление лишь не доставлено")
	}
}

func TestNewStoreWithoutPathSkipsDisk(t *testing.T) {
	store := NewStore("", nil, nil)
	if _, err := store.Submit(Order{Name: "А", Contact: "@a"}, nil); err != nil {
		t.Fatalf("заявка без файла не принята: %v", err)
	}
}

func TestNewTelegramNotifierNeedsBothSettings(t *testing.T) {
	if NewTelegramNotifier("", "123") != nil {
		t.Error("нотификатор создан без токена")
	}
	if NewTelegramNotifier("token", "") != nil {
		t.Error("нотификатор создан без чата")
	}
	if NewTelegramNotifier("token", "123") == nil {
		t.Error("нотификатор не создан при полных настройках")
	}
}

func TestTelegramNotifierSendsMessage(t *testing.T) {
	var gotPath, gotText, gotChat string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotPath = r.URL.Path
		gotText = r.PostFormValue("text")
		gotChat = r.PostFormValue("chat_id")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewTelegramNotifier("secret-token", "42")
	n.APIBase = srv.URL

	err := n.Notify(Order{ID: "KP-1", Name: "Дмитрий", Contact: "@d", Product: "sunhood", Message: "нужен козырёк"}, nil)
	if err != nil {
		t.Fatalf("отправка не удалась: %v", err)
	}
	if gotPath != "/botsecret-token/sendMessage" {
		t.Errorf("запрос ушёл на %q", gotPath)
	}
	if gotChat != "42" {
		t.Errorf("chat_id = %q", gotChat)
	}
	for _, want := range []string{"KP-1", "Дмитрий", "@d", "sunhood", "нужен козырёк"} {
		if !strings.Contains(gotText, want) {
			t.Errorf("в сообщении нет %q: %q", want, gotText)
		}
	}
}

func TestTelegramNotifierReportsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"ok":false,"description":"chat not found"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	n := NewTelegramNotifier("t", "1")
	n.APIBase = srv.URL

	err := n.Notify(Order{ID: "KP-1", Name: "А", Contact: "@a"}, nil)
	if err == nil {
		t.Fatal("ошибка Bot API проигнорирована")
	}
	if !strings.Contains(err.Error(), "chat not found") {
		t.Errorf("в ошибке нет ответа API: %v", err)
	}
}

// discardLogger гасит ожидаемые сообщения об ошибках в выводе тестов.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
