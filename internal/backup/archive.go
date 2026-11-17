package backup

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// В архиве файл заявок всегда лежит под этим именем, а вложения — в uploads/:
// так бэкап переживает смену ORDERS_FILE.
const (
	ordersEntry = "orders.jsonl"
	uploadsDir  = "uploads"
)

// countOrders — число заявок (полных строк) в файле; нет файла — 0.
func countOrders(ordersPath string) (int, error) {
	f, err := os.Open(ordersPath)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if err == io.EOF {
			return n, nil // недописанная последняя строка не считается
		}
		if err != nil {
			return n, err
		}
		if len(bytes.TrimSpace(line)) > 0 {
			n++
		}
	}
}

// pack пишет tar.gz с файлом заявок и каталогом вложений во временный файл
// и возвращает его вместе с hex SHA-256. Архив детерминирован (порядок
// файлов, нулевые владельцы и время gzip), поэтому одинаковые данные дают
// одинаковую сумму и повторная выгрузка не нужна.
func pack(ordersPath string) (f *os.File, size int64, sum string, err error) {
	f, err = os.CreateTemp("", "kinoperiferia-backup-*.tar.gz")
	if err != nil {
		return nil, 0, "", err
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(f.Name())
			f = nil
		}
	}()

	h := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(f, h))
	tw := tar.NewWriter(gz)

	if data, rerr := os.ReadFile(ordersPath); rerr == nil {
		// Отрезаем строку, которую сайт мог дописывать в этот момент.
		if i := bytes.LastIndexByte(data, '\n'); i >= 0 {
			data = data[:i+1]
		} else {
			data = nil
		}
		var mod time.Time
		if st, serr := os.Stat(ordersPath); serr == nil {
			mod = st.ModTime()
		}
		if err = writeEntry(tw, ordersEntry, mod, bytes.NewReader(data), int64(len(data))); err != nil {
			return
		}
	} else if !errors.Is(rerr, fs.ErrNotExist) {
		err = rerr
		return
	}

	root := filepath.Join(filepath.Dir(ordersPath), uploadsDir)
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			if errors.Is(werr, fs.ErrNotExist) && p == root {
				return fs.SkipDir
			}
			return werr
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, rerr := filepath.Rel(filepath.Dir(ordersPath), p)
		if rerr != nil {
			return rerr
		}
		st, serr := d.Info()
		if serr != nil {
			return serr
		}
		src, oerr := os.Open(p)
		if oerr != nil {
			return oerr
		}
		defer src.Close()
		return writeEntry(tw, filepath.ToSlash(rel), st.ModTime(), src, st.Size())
	})
	if err != nil {
		return
	}
	if err = tw.Close(); err != nil {
		return
	}
	if err = gz.Close(); err != nil {
		return
	}
	if size, err = f.Seek(0, io.SeekCurrent); err != nil {
		return
	}
	return f, size, hex.EncodeToString(h.Sum(nil)), nil
}

func writeEntry(tw *tar.Writer, name string, mod time.Time, r io.Reader, size int64) error {
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     name,
		Mode:     0o644,
		Size:     size,
		ModTime:  mod.UTC().Truncate(time.Second),
	}); err != nil {
		return err
	}
	_, err := io.CopyN(tw, r, size)
	return err
}

// unpack восстанавливает данные из архива: распаковывает его рядом с
// ordersPath во временный каталог и только потом подменяет файл заявок
// и каталог вложений — битый архив текущие данные не трогает.
func unpack(r io.Reader, ordersPath string) error {
	dataDir := filepath.Dir(ordersPath)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(dataDir, ".restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("архив повреждён: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("архив повреждён: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := path.Clean(hdr.Name)
		if name != ordersEntry && !strings.HasPrefix(name, uploadsDir+"/") {
			continue // в архиве ничего, кроме заявок и вложений, быть не должно
		}
		dst := filepath.Join(tmp, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, tr)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		// Время изменения как в бэкапе — тогда следующий архив совпадёт
		// с облачным, и сразу после восстановления выгружать нечего.
		_ = os.Chtimes(dst, hdr.ModTime, hdr.ModTime)
	}

	// Подмена: старое — в сторону, новое — на место.
	uploads := filepath.Join(dataDir, uploadsDir)
	if err := os.RemoveAll(uploads); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(tmp, uploadsDir), uploads); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	err = os.Rename(filepath.Join(tmp, ordersEntry), ordersPath)
	if errors.Is(err, fs.ErrNotExist) {
		// В бэкапе заявок не было — значит, и локально их быть не должно.
		err = os.Remove(ordersPath)
		if errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
	}
	return err
}
