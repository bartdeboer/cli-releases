package cliget

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

func safeDir(dir string) (string, error) {
	a, e := filepath.Abs(dir)
	if e != nil {
		return "", e
	}
	cur := string(filepath.Separator)
	for _, p := range splitPath(a) {
		cur = filepath.Join(cur, p)
		i, e := os.Lstat(cur)
		if e != nil || i.Mode()&os.ModeSymlink != 0 || !i.IsDir() {
			return "", errors.New("bin directory path must contain only real directories")
		}
	}
	i, _ := os.Stat(a)
	if st, ok := i.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return "", errors.New("bin directory must be owned by invoking user")
	}
	if i.Mode().Perm()&0022 != 0 {
		return "", errors.New("bin directory must not be group/world writable")
	}
	return a, nil
}
func splitPath(a string) []string {
	v := []string{}
	for {
		d, b := filepath.Split(a)
		if b != "" {
			v = append([]string{b}, v...)
		}
		a = filepath.Clean(d)
		if a == string(filepath.Separator) || a == "." {
			return v
		}
	}
}
func installFile(dir, name string, data []byte, overwrite bool) (string, error) {
	d, e := safeDir(dir)
	if e != nil {
		return "", e
	}
	dst := filepath.Join(d, name)
	if i, e := os.Lstat(dst); e == nil {
		if i.Mode()&os.ModeSymlink != 0 || !i.Mode().IsRegular() {
			return "", errors.New("destination is not a regular file")
		}
		if st, ok := i.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
			return "", errors.New("destination is not owned by invoking user")
		}
		if !overwrite {
			return "", errors.New("destination exists; --overwrite required")
		}
	} else if !os.IsNotExist(e) {
		return "", e
	}
	tmp := filepath.Join(d, "."+name+".tmp-"+strconv.Itoa(os.Getpid()))
	f, e := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if e != nil {
		return "", e
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(tmp)
		}
	}()
	if _, e = f.Write(data); e != nil {
		return "", e
	}
	if e = f.Sync(); e != nil {
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	if overwrite {
		e = os.Rename(tmp, dst)
	} else {
		e = os.Link(tmp, dst)
		if e == nil {
			e = os.Remove(tmp)
		}
	}
	if e != nil {
		return "", fmt.Errorf("atomic install failed: %w", e)
	}
	ok = true
	return dst, nil
}
