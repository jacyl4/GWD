package core

import (
	"io"
	"os"
)

// FileSystem abstracts filesystem operations to improve testability.
type FileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	Remove(path string) error
	Rename(oldPath, newPath string) error
	Chmod(path string, mode os.FileMode) error
	Stat(path string) (os.FileInfo, error)
	Open(path string) (io.ReadCloser, error)
	Create(path string) (io.WriteCloser, error)
}

// OSFileSystem implements FileSystem using the local OS.
type OSFileSystem struct{}

func (OSFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (OSFileSystem) Remove(path string) error {
	return os.Remove(path)
}

func (OSFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (OSFileSystem) Chmod(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}

func (OSFileSystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (OSFileSystem) Open(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (OSFileSystem) Create(path string) (io.WriteCloser, error) {
	return os.Create(path)
}
