package resourcejournal

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const directoryMarker = ".seven-cleanup-owner"

// TrackDirectory registers only a caller-created private scratch directory.
// Never pass retained workspace/result/review evidence roots here. Containers
// mounting children are dependencies and must be confirmed absent first.
func (j *Journal) TrackDirectory(path string, labels map[string]string) (Resource, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return Resource{}, errors.New("CLEANUP_DIRECTORY_NOT_PRIVATE")
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return Resource{}, err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return Resource{}, err
	}
	r := Resource{Key: hex.EncodeToString(nonce[:]), Kind: "directory", Name: path, Path: path, Labels: map[string]string{}}
	for k, v := range labels {
		r.Labels[k] = v
	}
	r.Labels[OwnershipLabel] = r.Key
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, old := range j.rows {
		if old.Path == path {
			return old, nil
		}
	}
	if j.fault {
		return Resource{}, errors.New("CLEANUP_JOURNAL_UNHEALTHY")
	}
	// Exclusive marker creation prevents adopting any previous owner's directory.
	f, err := os.OpenFile(filepath.Join(path, directoryMarker), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Resource{}, err
	}
	_, err = f.WriteString(r.Key)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return Resource{}, err
	}
	return r, j.save(r)
}

func (j *Journal) linkMounts(key string, args []string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, row := range j.rows {
		if row.Kind != "directory" {
			continue
		}
		matched := false
		for _, arg := range args {
			for _, part := range strings.Split(arg, ",") {
				source, ok := strings.CutPrefix(part, "src=")
				if !ok {
					continue
				}
				if source == row.Path || strings.HasPrefix(source, row.Path+string(filepath.Separator)) {
					matched = true
				}
			}
		}
		if matched {
			row.DependsOn = append(row.DependsOn, key)
			if err := j.save(row); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeDirectory(r Resource) error {
	info, err := os.Lstat(r.Path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("CLEANUP_DIRECTORY_IDENTITY_CONFLICT")
	}
	canonical, err := filepath.EvalSymlinks(r.Path)
	if err != nil || canonical != r.Path {
		return errors.New("CLEANUP_DIRECTORY_IDENTITY_CONFLICT")
	}
	marker := filepath.Join(r.Path, directoryMarker)
	info, err = os.Lstat(marker)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("CLEANUP_DIRECTORY_OWNER_CONFLICT")
	}
	body, err := os.ReadFile(marker)
	if err != nil || string(body) != r.Key {
		return errors.New("CLEANUP_DIRECTORY_OWNER_CONFLICT")
	}
	if err = os.RemoveAll(r.Path); err != nil {
		return errors.New("CLEANUP_DIRECTORY_REMOVE_FAILED")
	}
	if _, err = os.Lstat(r.Path); !os.IsNotExist(err) {
		return errors.New("CLEANUP_DIRECTORY_ABSENCE_UNCONFIRMED")
	}
	return nil
}
