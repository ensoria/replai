package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	idPrefix      = "s"
	idRandomBytes = 4

	stateExt = ".json"
	logExt   = ".jsonl"
	lockExt  = ".lock"

	filePerm = 0o644
)

// Store persists sessions as JSON files under <project>/.replai/sessions.
type Store struct {
	Dir string
}

// NewStore returns a store rooted at dir.
func NewStore(dir string) *Store {
	return &Store{Dir: dir}
}

// Create allocates a new session.
func (s *Store) Create(projectDir string) (*State, error) {
	buf := make([]byte, idRandomBytes)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	st := &State{
		ID:         idPrefix + hex.EncodeToString(buf),
		CreatedAt:  time.Now().UTC(),
		ProjectDir: projectDir,
		Entries:    []*Entry{},
	}
	if err := s.Save(st); err != nil {
		return nil, err
	}
	return st, nil
}

// Load reads a session state.
func (s *Store) Load(id string) (*State, error) {
	data, err := os.ReadFile(s.statePath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session %q not found; run `replai session start` first", id)
		}
		return nil, err
	}
	st := &State{}
	if err := json.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("session %q is corrupted: %w", id, err)
	}
	return st, nil
}

// Save writes a session state atomically.
func (s *Store) Save(st *State) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := s.statePath(st.ID) + ".tmp"
	if err := os.WriteFile(tmp, data, filePerm); err != nil {
		return err
	}
	return os.Rename(tmp, s.statePath(st.ID))
}

// Delete removes the session state, log, and lock files.
func (s *Store) Delete(id string) error {
	if _, err := os.Stat(s.statePath(id)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("session %q not found", id)
		}
		return err
	}
	for _, p := range []string{s.statePath(id), s.logPath(id), s.lockPath(id)} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// WithLock loads the session under an exclusive flock, runs fn, and saves the
// (possibly mutated) state. Concurrent evals against the same session from
// parallel agent processes serialize here.
func (s *Store) WithLock(id string, fn func(st *State) error) error {
	return s.withLock(id, func(st *State) error {
		if err := fn(st); err != nil {
			return err
		}
		return s.Save(st)
	})
}

// WithLockAndLog loads the session under an exclusive flock, runs fn, saves
// the mutated state, then appends the returned log record before releasing the
// lock. This keeps session logs in the same order as serialized evals.
func (s *Store) WithLockAndLog(id string, fn func(st *State) (*LogRecord, error)) error {
	return s.withLock(id, func(st *State) error {
		rec, err := fn(st)
		if err != nil {
			return err
		}
		if err := s.Save(st); err != nil {
			return err
		}
		if rec == nil {
			return nil
		}
		return s.appendLog(id, rec)
	})
}

func (s *Store) withLock(id string, fn func(st *State) error) error {
	lock, err := os.OpenFile(s.lockPath(id), os.O_CREATE|os.O_RDWR, filePerm)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()

	st, err := s.Load(id)
	if err != nil {
		return err
	}
	if err := fn(st); err != nil {
		return err
	}
	return nil
}

// LogRecord is one line of the session JSONL log.
type LogRecord struct {
	Time   time.Time       `json:"time"`
	Input  string          `json:"input"`
	Output json.RawMessage `json:"output"`
}

// AppendLog appends one record to the session log.
func (s *Store) AppendLog(id string, rec *LogRecord) error {
	return s.appendLog(id, rec)
}

func (s *Store) appendLog(id string, rec *LogRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.logPath(id), os.O_CREATE|os.O_APPEND|os.O_WRONLY, filePerm)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

// ReadLog returns all log records of a session.
func (s *Store) ReadLog(id string) ([]json.RawMessage, error) {
	data, err := os.ReadFile(s.logPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return []json.RawMessage{}, nil
		}
		return nil, err
	}
	var out []json.RawMessage
	for _, line := range splitLines(data) {
		if len(line) > 0 {
			out = append(out, json.RawMessage(line))
		}
	}
	return out, nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

func (s *Store) statePath(id string) string { return filepath.Join(s.Dir, id+stateExt) }
func (s *Store) logPath(id string) string   { return filepath.Join(s.Dir, id+logExt) }
func (s *Store) lockPath(id string) string  { return filepath.Join(s.Dir, id+lockExt) }
