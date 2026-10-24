// Package interactionspool stores opaque immutable interaction frames in a
// Worker-private directory. Never mount this directory into an Attempt. Local
// persistence is not Server acceptance; only AcknowledgeServer can remove data.
package interactionspool

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

var (
	ErrInvalid      = errors.New("INTERACTION_SPOOL_INVALID")
	ErrIO           = errors.New("INTERACTION_SPOOL_IO")
	ErrFull         = errors.New("INTERACTION_SPOOL_FULL")
	ErrConflict     = errors.New("INTERACTION_SPOOL_CONFLICT")
	ErrUnauthorized = errors.New("INTERACTION_SPOOL_SERVER_ACK_REQUIRED")
	ErrClosed       = errors.New("INTERACTION_SPOOL_CLOSED")
)

// Identity is frozen by the authenticated execution owner, never derived from
// the container frame. Round and session generation fence resumed execution.
type Identity struct {
	TaskID            int64  `json:"taskId,string"`
	AttemptID         int64  `json:"attemptId,string"`
	Generation        int64  `json:"generation,string"`
	OwnerEpoch        int64  `json:"ownerEpoch,string"`
	CurrentRound      string `json:"currentRound"`
	SessionGeneration int64  `json:"sessionGeneration,string"`
}

type Key struct {
	Identity   Identity `json:"identity"`
	Occurrence string   `json:"occurrence"`
}
type Receipt struct {
	Key    Key    `json:"key"`
	SHA256 string `json:"sha256"`
}
type Entry struct {
	Receipt Receipt `json:"receipt"`
	Frame   []byte  `json:"frame"`
}
type Limits struct {
	MaxEntries    int
	MaxBytes      int64
	MaxFrameBytes int
}

// ServerAckVerifier must authenticate the Server response and bind it to the
// exact receipt (identity, occurrence AND digest). The proof is opaque here.
// It is invoked only by AcknowledgeServer, never by Put, Replay or a frame parser.
type ServerAckVerifier func(context.Context, Receipt, []byte) error

type Spool struct {
	mu      sync.Mutex
	dir     *os.File
	limits  Limits
	verify  ServerAckVerifier
	entries map[string]stored
	bytes   int64
	failed  bool
	// Per-handle persistence operations also permit deterministic crash-boundary
	// fault tests without global hooks or weakening the public API.
	syncFile      func(*os.File) error
	syncDirectory func() error
	rename        func(string, string) error
}
type stored struct {
	receipt Receipt
	size    int64
}

func validKey(k Key) bool {
	i := k.Identity
	if i.TaskID <= 0 || i.AttemptID <= 0 || i.Generation <= 0 || i.OwnerEpoch <= 0 || i.SessionGeneration <= 0 || !validOpaqueKey(i.CurrentRound) {
		return false
	}
	return validOpaqueKey(k.Occurrence)
}

func validOpaqueKey(key string) bool {
	if len(key) == 0 || len(key) > 128 {
		return false
	}
	for _, c := range key {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._:-", c)) {
			return false
		}
	}
	return true
}
func nameFor(k Key) string {
	b, _ := json.Marshal(k)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]) + ".frame"
}
func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

// Open creates the leaf directory if absent. Every path component must be a
// real directory (no symlinks or '..'); the leaf must be owned by this uid and
// private (0700). An exclusive nonblocking directory lock prevents two writers.
// All subsequent operations use the pinned directory fd, not the original path.
func Open(path string, limits Limits, verify ServerAckVerifier) (*Spool, error) {
	if limits.MaxEntries <= 0 || limits.MaxEntries > 100000 || limits.MaxFrameBytes <= 0 || limits.MaxFrameBytes > 1<<20 || limits.MaxBytes <= 0 || limits.MaxBytes > 1<<40 {
		return nil, ErrInvalid
	}
	dir, err := openDirectory(path)
	if err != nil {
		return nil, err
	}
	s := &Spool{dir: dir, limits: limits, verify: verify, entries: make(map[string]stored)}
	s.syncFile = func(f *os.File) error { return f.Sync() }
	s.syncDirectory = dir.Sync
	s.rename = func(from, to string) error { return unix.Renameat(int(dir.Fd()), from, int(dir.Fd()), to) }
	if err = s.load(); err != nil {
		_ = dir.Close()
		return nil, err
	}
	return s, nil
}

func openDirectory(path string) (*os.File, error) {
	if !filepath.IsAbs(path) || path == "/" || strings.IndexByte(path, 0) >= 0 {
		return nil, ErrInvalid
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, ErrInvalid
		}
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrIO
	}
	for index, p := range parts {
		next, e := unix.Openat(fd, p, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e == unix.ENOENT && index == len(parts)-1 {
			if e = unix.Mkdirat(fd, p, 0700); e == nil {
				e = unix.Fsync(fd)
			}
			if e == nil {
				next, e = unix.Openat(fd, p, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			}
		}
		_ = unix.Close(fd)
		if e != nil {
			return nil, ErrInvalid
		}
		fd = next
	}
	f := os.NewFile(uintptr(fd), "interaction-spool")
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&0777 != 0700 || int(st.Uid) != os.Geteuid() {
		_ = f.Close()
		return nil, ErrInvalid
	}
	if unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil {
		_ = f.Close()
		return nil, ErrIO
	}
	return f, nil
}

func (s *Spool) openFile(name string) (*os.File, error) {
	fd, err := unix.Openat(int(s.dir.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrIO
	}
	f := os.NewFile(uintptr(fd), "interaction-frame")
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0777 != 0600 || st.Nlink != 1 || int(st.Uid) != os.Geteuid() {
		_ = f.Close()
		return nil, ErrInvalid
	}
	return f, nil
}

func (s *Spool) read(name string) (Entry, int64, error) {
	var entry Entry
	f, err := s.openFile(name)
	if err != nil {
		return entry, 0, err
	}
	defer f.Close()
	max := int64(s.limits.MaxFrameBytes)*2 + 4096
	info, err := f.Stat()
	if err != nil || info.Size() > max {
		return entry, 0, ErrInvalid
	}
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || int64(len(b)) > max {
		return entry, 0, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&entry) != nil || !validKey(entry.Receipt.Key) || len(entry.Frame) == 0 || len(entry.Frame) > s.limits.MaxFrameBytes || entry.Receipt.SHA256 != digest(entry.Frame) || nameFor(entry.Receipt.Key) != name {
		return Entry{}, 0, ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return Entry{}, 0, ErrInvalid
	}
	canonical, _ := json.Marshal(entry)
	if !bytes.Equal(b, canonical) {
		return Entry{}, 0, ErrInvalid
	}
	return entry, int64(len(b)), nil
}

func (s *Spool) load() error {
	// Bound directory enumeration as well as decoded bytes, including crash temps.
	files, err := s.dir.ReadDir(s.limits.MaxEntries + 2)
	if err != nil && err != io.EOF {
		return ErrIO
	}
	if len(files) > s.limits.MaxEntries+1 {
		return ErrFull
	}
	for _, file := range files {
		name := file.Name()
		if strings.HasPrefix(name, ".pending-") {
			if len(name) != len(".pending-")+32 {
				return ErrInvalid
			}
			if _, err := hex.DecodeString(strings.TrimPrefix(name, ".pending-")); err != nil {
				return ErrInvalid
			}
			f, err := s.openFile(name)
			if err != nil {
				return err
			}
			_ = f.Close()
			if unix.Unlinkat(int(s.dir.Fd()), name, 0) != nil {
				return ErrIO
			}
			continue // uncommitted temp; Put never returned a receipt for it
		}
		if len(name) != 70 || !strings.HasSuffix(name, ".frame") {
			return ErrInvalid
		}
		entry, size, err := s.read(name)
		if err != nil {
			return err
		}
		if len(s.entries) >= s.limits.MaxEntries || size > s.limits.MaxBytes-s.bytes {
			return ErrFull
		}
		s.entries[name] = stored{receipt: entry.Receipt, size: size}
		s.bytes += size
	}
	if s.dir.Sync() != nil {
		return ErrIO
	}
	return nil
}

func (s *Spool) usable() error {
	if s.dir == nil {
		return ErrClosed
	}
	if s.failed {
		return ErrIO
	}
	return nil
}

// Put returns only after atomic rename and both file and directory fsync.
// Duplicate identical entries converge; differing bytes at the same key fail.
// Capacity counts encoded on-disk bytes, including the envelope. I/O uncertainty
// poisons the handle: close/reopen reconciles disk before any further operation.
func (s *Spool) Put(ctx context.Context, key Key, frame []byte) (Receipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var empty Receipt
	if err := s.usable(); err != nil {
		return empty, err
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if !validKey(key) || len(frame) == 0 || len(frame) > s.limits.MaxFrameBytes {
		return empty, ErrInvalid
	}
	entry := Entry{Receipt: Receipt{Key: key, SHA256: digest(frame)}, Frame: append([]byte(nil), frame...)}
	name := nameFor(key)
	if stored, ok := s.entries[name]; ok {
		old, _, err := s.read(name)
		if err != nil {
			s.failed = true
			return empty, err
		}
		if old.Receipt != entry.Receipt || !bytes.Equal(old.Frame, frame) {
			return empty, ErrConflict
		}
		if old.Receipt != stored.receipt {
			s.failed = true
			return empty, ErrConflict
		}
		return old.Receipt, nil
	}
	body, _ := json.Marshal(entry)
	if len(s.entries) >= s.limits.MaxEntries || int64(len(body)) > s.limits.MaxBytes-s.bytes {
		return empty, ErrFull
	}
	// Refuse an unexpected destination (including a symlink) before rename.
	var st unix.Stat_t
	if err := unix.Fstatat(int(s.dir.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW); err != unix.ENOENT {
		s.failed = true
		return empty, ErrInvalid
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return empty, ErrIO
	}
	tmp := fmt.Sprintf(".pending-%x", nonce)
	fd, err := unix.Openat(int(s.dir.Fd()), tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		s.failed = true
		return empty, ErrIO
	}
	f := os.NewFile(uintptr(fd), "interaction-pending")
	n, writeErr := f.Write(body)
	syncErr := s.syncFile(f)
	closeErr := f.Close()
	if writeErr != nil || n != len(body) || syncErr != nil || closeErr != nil {
		s.failed = true
		return empty, ErrIO
	}
	if s.rename(tmp, name) != nil || s.syncDirectory() != nil {
		s.failed = true
		return empty, ErrIO
	}
	s.entries[name] = stored{receipt: entry.Receipt, size: int64(len(body))}
	s.bytes += int64(len(body))
	return entry.Receipt, nil
}

// Replay returns fresh byte copies in lexicographic occurrence order for one
// exact identity. No frame is parsed for commands or acknowledgements.
func (s *Spool) Replay(ctx context.Context, identity Identity) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.usable(); err != nil {
		return nil, err
	}
	if !validKey(Key{Identity: identity, Occurrence: "check"}) {
		return nil, ErrInvalid
	}
	var entries []Entry
	for name, stored := range s.entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if stored.receipt.Key.Identity != identity {
			continue
		}
		entry, _, err := s.read(name)
		if err != nil {
			s.failed = true
			return nil, err
		}
		if entry.Receipt != stored.receipt {
			s.failed = true
			return nil, ErrConflict
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Receipt.Key.Occurrence < entries[j].Receipt.Key.Occurrence })
	return entries, nil
}

// Pending lists bounded receipt metadata without loading frame bodies. A Worker
// hub can select an execution, then Read each immutable frame for restart replay.
func (s *Spool) Pending(ctx context.Context) ([]Receipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.usable(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]Receipt, 0, len(s.entries))
	for _, entry := range s.entries {
		result = append(result, entry.receipt)
	}
	sort.Slice(result, func(i, j int) bool { return nameFor(result[i].Key) < nameFor(result[j].Key) })
	return result, nil
}

func (s *Spool) Read(ctx context.Context, receipt Receipt) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.usable(); err != nil {
		return Entry{}, err
	}
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	name := nameFor(receipt.Key)
	stored, ok := s.entries[name]
	if !ok || stored.receipt != receipt {
		return Entry{}, ErrConflict
	}
	entry, _, err := s.read(name)
	if err != nil {
		s.failed = true
		return Entry{}, err
	}
	if entry.Receipt != receipt {
		s.failed = true
		return Entry{}, ErrConflict
	}
	return entry, nil
}

// AcknowledgeServer is the sole committed-entry deletion API. Configure verify
// from trusted Worker wiring; it must reject container/local receipts. Proof is
// bounded, never persisted or logged. Failed verification always retains data.
func (s *Spool) AcknowledgeServer(ctx context.Context, receipt Receipt, proof []byte) error {
	if !validKey(receipt.Key) || len(receipt.SHA256) != 64 || len(proof) == 0 || len(proof) > 64<<10 {
		return ErrInvalid
	}
	if s.verify == nil {
		return ErrUnauthorized
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.verify(ctx, receipt, append([]byte(nil), proof...)) != nil {
		return ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.usable(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	name := nameFor(receipt.Key)
	stored, ok := s.entries[name]
	if !ok {
		return nil
	} // authenticated duplicate Server acknowledgement
	if stored.receipt != receipt {
		return ErrConflict
	}
	entry, _, err := s.read(name)
	if err != nil {
		s.failed = true
		return err
	}
	if entry.Receipt != receipt {
		return ErrConflict
	}
	if unix.Unlinkat(int(s.dir.Fd()), name, 0) != nil || s.syncDirectory() != nil {
		s.failed = true
		return ErrIO
	}
	delete(s.entries, name)
	s.bytes -= stored.size
	return nil
}

func (s *Spool) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == nil {
		return nil
	}
	err := s.dir.Close()
	s.dir = nil
	if err != nil {
		return ErrIO
	}
	return nil
}
