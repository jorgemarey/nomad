package writer

import (
	"bufio"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"strings"
	"sync"
	"time"

	omega "globaldevtools.bbva.com/entsec/semaas/omega/api"
	omegaLoader "globaldevtools.bbva.com/entsec/semaas/omega/loader"
	rho "globaldevtools.bbva.com/entsec/semaas/rho/api"
	rhoLoader "globaldevtools.bbva.com/entsec/semaas/rho/loader"
)

const (
	maxLineSize = 32 * 1024
)

// Writer defines an omega writer. Currently is just an "alias" to io.WriteCloser
type Writer interface {
	io.WriteCloser
}

type writer struct {
	rhoLoader   *rhoLoader.Loader
	omegaLoader *omegaLoader.Loader
	level       omega.LogLevel
	properties  map[string]interface{}
	mrid        string
	reader      *io.PipeReader
	writer      *io.PipeWriter
	scanner     *bufio.Scanner
	wg          sync.WaitGroup
	sysLogger   *log.Logger
}

func (w *writer) Write(p []byte) (n int, err error) {
	return w.writer.Write(p)
}

func (w *writer) Close() error {
	w.writer.Close()
	w.wg.Wait()
	return w.omegaLoader.Close()
}

// New returns a configured writer that sends logs using the provided logader
func New(ol *omegaLoader.Loader, rl *rhoLoader.Loader, level omega.LogLevel, properties map[string]interface{}, mrid string, sysLogger *log.Logger) Writer {
	pr, pw := io.Pipe()
	w := &writer{
		rhoLoader:   rl,
		omegaLoader: ol,
		level:       level,
		properties:  properties,
		mrid:        mrid,
		reader:      pr,
		writer:      pw,
		sysLogger:   sysLogger,
	}
	if w.sysLogger == nil {
		w.sysLogger = log.New(ioutil.Discard, "", log.LstdFlags)
	}
	w.scanner = bufio.NewScanner(pr)
	w.scanner.Split(sizeSpliter(maxLineSize, bufio.ScanLines))
	go w.scan()
	return w
}

func (w *writer) scan() {
	w.wg.Add(1)
	defer w.wg.Done()
	for w.scanner.Scan() {
		text := w.scanner.Text()
		if ok := w.processLine(text); !ok {
			le := &omega.LogEntry{
				MrID:         w.mrid,
				Message:      text,
				CreationDate: time.Now().UnixNano(),
				Properties:   w.properties,
				Level:        w.level,
			}
			if err := w.omegaLoader.Load(le); err != nil {
				w.sysLogger.Printf("[ERR] Unable to load message: %s", err)
			}
		}
	}
	if w.scanner.Err() != nil {
		// We need to keep reading even if the scaner fails.
		// All the reader data must be consumed
		io.Copy(ioutil.Discard, w.reader)
	}
}

// sizeSpliter wraps a bufio.SplitFunc by limiting the max size of the split.
// This allow us not to use too much memory and avoid the bufio.ErrTooLong.

// This works by checking the output of the SplitFunc. If no token is found
// and the provided buffer length is greater than maxSize it will be returned
// as a new token. If a SplitFunc finds a token earlier, that token will be
// returned.
func sizeSpliter(maxSize int, splitFunc bufio.SplitFunc) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		advance, token, err = splitFunc(data, atEOF)
		if err != nil {
			return
		}
		if advance == 0 && token == nil && len(data) > maxSize {
			advance = len(data)
			token = data
		}
		return
	}
}

func (w *writer) processLine(text string) bool {
	if !strings.HasPrefix(text, "V2|") {
		return false
	}
	parts := strings.Split(text, "|")
	if len(parts) < 3 {
		return false
	}
	kind := parts[1]
	data := strings.Join(parts[2:], "|")
	switch {
	case strings.HasPrefix(kind, "LOG"): // it is a log
		var entry omega.LogEntry
		if err := json.NewDecoder(strings.NewReader(data)).Decode(&entry); err != nil {
			return false
		}
		// set the level
		if entry.Level == "" {
			levelParts := strings.Split(kind, ".")
			entry.Level = w.level
			if len(levelParts) > 1 {
				entry.Level = omega.LogLevel(levelParts[1])
			}
		}
		// we add the custom properties here
		if entry.Properties == nil {
			entry.Properties = make(map[string]interface{})
		}
		for k, v := range w.properties {
			entry.Properties[k] = v
		}
		if err := w.omegaLoader.Load(&entry); err != nil {
			w.sysLogger.Printf("[ERR] Unable to load message: %s", err)
		}
	case strings.HasPrefix(kind, "SPAN"): // it is a span
		var span rho.Span
		if err := json.NewDecoder(strings.NewReader(data)).Decode(&span); err != nil {
			return false
		}
		// we add the custom properties here
		if span.Properties == nil {
			span.Properties = make(map[string]interface{})
		}
		for k, v := range w.properties {
			span.Properties[k] = v
		}
		if err := w.rhoLoader.Load(&span); err != nil {
			w.sysLogger.Printf("[ERR] Unable to load span: %s", err)
		}
	default:
		return false
	}
	return true
}
