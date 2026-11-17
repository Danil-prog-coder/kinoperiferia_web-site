package backup

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// errNotFound — объекта в бакете нет (бэкап ещё ни разу не снимался).
var errNotFound = errors.New("объект не найден")

// s3Client — минимальный клиент S3 (HEAD/GET/PUT одного объекта) с подписью
// AWS Signature V4, path-style адреса. Хватает для Selectel и любого
// S3-совместимого хранилища, а сайт остаётся на одной стандартной библиотеке.
type s3Client struct {
	endpoint  string // https://s3.ru-6.storage.selcloud.ru
	region    string
	bucket    string
	accessKey string
	secretKey string
	http      *http.Client
	now       func() time.Time
}

func (c *s3Client) objectURL(key string) (*url.URL, error) {
	segs := strings.Split(c.bucket+"/"+key, "/")
	for i, s := range segs {
		segs[i] = uriEncode(s)
	}
	return url.Parse(strings.TrimRight(c.endpoint, "/") + "/" + strings.Join(segs, "/"))
}

// head возвращает пользовательские метаданные объекта (x-amz-meta-*),
// ключи — в нижнем регистре без префикса.
func (c *s3Client) head(ctx context.Context, key string) (map[string]string, error) {
	resp, err := c.do(ctx, http.MethodHead, key, nil, 0, emptySHA256, nil)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	meta := map[string]string{}
	for k, v := range resp.Header {
		if lk := strings.ToLower(k); strings.HasPrefix(lk, "x-amz-meta-") && len(v) > 0 {
			meta[strings.TrimPrefix(lk, "x-amz-meta-")] = v[0]
		}
	}
	return meta, nil
}

// get скачивает объект в w.
func (c *s3Client) get(ctx context.Context, key string, w io.Writer) error {
	resp, err := c.do(ctx, http.MethodGet, key, nil, 0, emptySHA256, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(w, resp.Body)
	return err
}

// put загружает файл целиком; sum — hex SHA-256 его содержимого.
func (c *s3Client) put(ctx context.Context, key string, f *os.File, size int64, sum string, meta map[string]string) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	h := map[string]string{"content-type": "application/gzip"}
	for k, v := range meta {
		h["x-amz-meta-"+strings.ToLower(k)] = v
	}
	resp, err := c.do(ctx, http.MethodPut, key, f, size, sum, h)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func (c *s3Client) do(ctx context.Context, method, key string, body io.Reader, size int64, payloadSum string, headers map[string]string) (*http.Response, error) {
	u, err := c.objectURL(key)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.ContentLength = size
		// *os.File не перематывается сам: без GetBody ретраи транспорта невозможны,
		// это нормально — повтор сделает следующий цикл бэкапа.
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.sign(req, payloadSum)

	client := c.http
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, errNotFound
	}
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("S3 %s %s: %s %s", method, key, resp.Status, strings.TrimSpace(string(msg)))
	}
	return resp, nil
}

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// sign добавляет заголовок Authorization по схеме AWS Signature V4.
func (c *s3Client) sign(req *http.Request, payloadSum string) {
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	t := now().UTC()
	amzDate := t.Format("20060102T150405Z")
	day := t.Format("20060102")

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadSum)

	// Подписываем host и все x-amz-*/content-type заголовки.
	signed := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-amz-") || lk == "content-type" {
			signed[lk] = strings.TrimSpace(strings.Join(v, ","))
		}
	}
	names := make([]string, 0, len(signed))
	for k := range signed {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, k := range names {
		canonHeaders.WriteString(k + ":" + signed[k] + "\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonical := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		req.URL.RawQuery,
		canonHeaders.String(),
		signedHeaders,
		payloadSum,
	}, "\n")

	scope := day + "/" + c.region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256Hex([]byte(canonical))

	key := hmacSHA256([]byte("AWS4"+c.secretKey), day)
	key = hmacSHA256(key, c.region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(key, toSign))

	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+c.accessKey+"/"+scope+
		", SignedHeaders="+signedHeaders+", Signature="+sig)
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// uriEncode кодирует сегмент пути так, как этого требует SigV4: всё, кроме
// A-Z a-z 0-9 - _ . ~, превращается в %XX с заглавными цифрами.
func uriEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
