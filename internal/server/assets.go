package server

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
)

// assetVersions хранит короткий хеш содержимого каждого статического файла,
// чтобы ссылки вида /static/css/style.css?v=1a2b3c обновлялись при изменении
// файла и при этом отдавались с длинным кешем.
type assetVersions map[string]string

func buildAssetVersions(fsys fs.FS) (assetVersions, error) {
	versions := assetVersions{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		versions["/static/"+p] = hex.EncodeToString(sum[:])[:10]
		return nil
	})
	return versions, err
}

// URL возвращает путь к файлу с версией. Неизвестные пути отдаются как есть —
// так опечатка в шаблоне не превращается в панику на проде.
func (a assetVersions) URL(p string) string {
	if v, ok := a[p]; ok {
		return p + "?v=" + v
	}
	return p
}

// staticHandler отдаёт встроенные файлы. Запрос с параметром v считается
// версионированным, поэтому кешируется на год. Шрифты подключаются из CSS
// без версии, но их файлы никогда не меняются под тем же именем — тоже на
// год. Фотографии каталога живут сутки и ещё неделю отдаются из кеша, пока
// браузер в фоне проверяет свежесть: повторные визиты не ждут сеть.
func staticHandler(fsys fs.FS) http.Handler {
	files := http.FileServer(http.FS(fsys))
	return http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Query().Get("v") != "", strings.HasPrefix(r.URL.Path, "fonts/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case strings.HasPrefix(r.URL.Path, "img/"):
			w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
		default:
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		files.ServeHTTP(w, r)
	}))
}
