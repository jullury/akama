package logger

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultMaxSize is the default maximum size of a log file before rotation (10 MB).
	DefaultMaxSize int64 = 10 << 20
	// archiveSuffix is the suffix for archived (compressed) log files.
	archiveSuffix = ".gz"
	// timeFormat is used for naming daily log files.
	timeFormat = "2006-01-02"
)

// Config holds logger configuration.
type Config struct {
	// LogPath is the base path for log files (e.g., ~/.akama/akama.log).
	// Daily files will be named akama-YYYY-MM-DD.log alongside this path.
	LogPath string
	// MaxSize is the maximum size in bytes before a log file is archived.
	// Defaults to DefaultMaxSize if zero.
	MaxSize int64
	// MaxArchives is the maximum number of archived files to keep.
	// Defaults to 7 if zero.
	MaxArchives int
}

// RotatingWriter implements io.Writer with daily rotation and size-based archival.
type RotatingWriter struct {
	cfg      Config
	mu       sync.Mutex
	file     *os.File
	curPath  string
	curDate  string
	curSize  int64
}

// NewRotatingWriter creates a new RotatingWriter.
// It opens (or creates) today's log file and starts writing to it.
func NewRotatingWriter(cfg Config) (*RotatingWriter, error) {
	if cfg.MaxSize == 0 {
		cfg.MaxSize = DefaultMaxSize
	}
	if cfg.MaxArchives == 0 {
		cfg.MaxArchives = 7
	}

	w := &RotatingWriter{cfg: cfg}
	if err := w.openCurrent(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *RotatingWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.shouldRotate() {
		if err := w.rotate(); err != nil {
			// Fall back to stderr if rotation fails
			fmt.Fprintf(os.Stderr, "log rotation failed: %v\n", err)
		}
	}

	n, err = w.file.Write(p)
	w.curSize += int64(n)
	return n, err
}

// shouldRotate returns true if the log should be rotated.
func (w *RotatingWriter) shouldRotate() bool {
	today := time.Now().Format(timeFormat)
	if today != w.curDate {
		return true
	}
	if w.cfg.MaxSize > 0 && w.curSize >= w.cfg.MaxSize {
		return true
	}
	return false
}

// rotate closes the current file, archives it if needed, and opens a new one.
func (w *RotatingWriter) rotate() error {
	// Close current file if open
	if w.file != nil {
		w.file.Close()
		w.file = nil
	}

	// Archive the current log file (compress it) if it exists and has content
	if w.curPath != "" {
		if info, err := os.Stat(w.curPath); err == nil && info.Size() > 0 {
			if err := w.archiveFile(w.curPath); err != nil {
				fmt.Fprintf(os.Stderr, "archive log %s: %v\n", w.curPath, err)
			}
		}
		// Remove the uncompressed file after archiving
		os.Remove(w.curPath)
	}

	// Clean up old archives beyond MaxArchives
	w.cleanOldArchives()

	return w.openCurrent()
}

// openCurrent opens (or creates) today's log file.
func (w *RotatingWriter) openCurrent() error {
	dir := filepath.Dir(w.cfg.LogPath)
	base := filepath.Base(w.cfg.LogPath)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]

	today := time.Now().Format(timeFormat)
	dirPath := filepath.Join(dir, "logs")
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	logPath := filepath.Join(dirPath, fmt.Sprintf("%s-%s%s", name, today, ext))
	w.curPath = logPath
	w.curDate = today

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("stat log file: %w", err)
	}

	w.file = f
	w.curSize = info.Size()
	return nil
}

// archiveFile compresses the given file and writes it with a .gz extension.
func (w *RotatingWriter) archiveFile(srcPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dstPath := srcPath + archiveSuffix
	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	gz := gzip.NewWriter(dst)
	defer gz.Close()

	_, err = io.Copy(gz, src)
	return err
}

// cleanOldArchives removes archived files beyond MaxArchives, keeping the most recent.
func (w *RotatingWriter) cleanOldArchives() {
	dir := filepath.Dir(w.curPath)
	base := filepath.Base(w.cfg.LogPath) // e.g. "akama.log"
	ext := filepath.Ext(base)              // e.g. ".log"
	name := base[:len(base)-len(ext)]     // e.g. "akama"

	// Collect all archive files matching: name-YYYY-MM-DD.ext.gz
	type archFile struct {
		path string
		info os.FileInfo
	}

	var archives []archFile
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		nameMatch := e.Name()
		// Must end with .gz and contain the base name pattern
		if !strings.HasSuffix(nameMatch, archiveSuffix) {
			continue
		}
		// Check it looks like our archive: name-YYYY-MM-DD.ext.gz
		withoutGz := strings.TrimSuffix(nameMatch, archiveSuffix)
		if !strings.HasPrefix(withoutGz, name+"-") || !strings.HasSuffix(withoutGz, ext) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		archives = append(archives, archFile{path: filepath.Join(dir, nameMatch), info: info})
	}

	if len(archives) <= w.cfg.MaxArchives {
		return
	}

	// Sort by modification time (oldest first)
	for i := 0; i < len(archives)-1; i++ {
		for j := i + 1; j < len(archives); j++ {
			if archives[i].info.ModTime().After(archives[j].info.ModTime()) {
				archives[i], archives[j] = archives[j], archives[i]
			}
		}
	}

	// Remove oldest archives beyond the limit
	for i := 0; i < len(archives)-w.cfg.MaxArchives; i++ {
		os.Remove(archives[i].path)
	}
}

// Close closes the underlying log file.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}
