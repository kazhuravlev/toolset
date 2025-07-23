package fsh

import (
	"os"
	"time"

	"github.com/spf13/afero"
)

const DefaultDirPerm = 0o755

type RealFs struct {
	fs *afero.OsFs
}

func NewRealFS() FS {
	return &RealFs{
		fs: afero.NewOsFs().(*afero.OsFs),
	}
}

func (r *RealFs) MkdirAll(path string, perm os.FileMode) error {
	return r.fs.MkdirAll(path, perm)
}

func (r *RealFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return r.fs.OpenFile(name, flag, perm)
}

func (r *RealFs) Remove(name string) error {
	return r.fs.Remove(name)
}

func (r *RealFs) RemoveAll(path string) error {
	return r.fs.RemoveAll(path)
}

func (r *RealFs) Stat(name string) (os.FileInfo, error) {
	return r.fs.Stat(name)
}

func (r *RealFs) SymlinkIfPossible(oldname, newname string) error {
	return r.fs.SymlinkIfPossible(oldname, newname)
}

func (r *RealFs) GetCurrentDir() string {
	dir, err := os.Getwd()
	if err != nil {
		// FIXME(zhuravlev): fix
		panic("current ir is not defined: " + err.Error())
	}

	return dir
}

func (r *RealFs) Name() string {
	return r.fs.Name()
}

func (r *RealFs) Chmod(name string, mode os.FileMode) error {
	return r.fs.Chmod(name, mode)
}

func (r *RealFs) Chown(name string, uid, gid int) error {
	return r.fs.Chown(name, uid, gid)
}

func (r *RealFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return r.fs.Chtimes(name, atime, mtime)
}

func (r *RealFs) Rename(oldname, newname string) error {
	return r.fs.Rename(oldname, newname)
}

func (r *RealFs) Create(name string) (afero.File, error) {
	return r.fs.Create(name)
}

func (r *RealFs) Mkdir(name string, perm os.FileMode) error {
	return r.fs.Mkdir(name, perm)
}

func (r *RealFs) Open(name string) (afero.File, error) {
	return r.fs.Open(name)
}
