package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"path/filepath"

	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/spf13/afero"
	"github.com/ulikunitz/xz"
)

func Extract(fs fsh.FS, archivePath, destDir string) error {
	f, err := fs.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	ext := fsh.Ext(archivePath)

	switch ext {
	case ".zip":
		return extractZip(fs, f, destDir)
	case ".tar.gz", ".tgz":
		return extractTar(fs, f, destDir, func(r io.Reader) (io.Reader, error) { return gzip.NewReader(r) })
	case ".tar.bz2":
		return extractTar(fs, f, destDir, func(r io.Reader) (io.Reader, error) { return bzip2.NewReader(r), nil })
	case ".tar.xz":
		return extractTar(fs, f, destDir, func(r io.Reader) (io.Reader, error) { return xz.NewReader(r) })
	}

	return fmt.Errorf("unsupported archive type (%s)", ext)
}

func extractZip(fs fsh.FS, f afero.File, dest string) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}

	r, err := zip.NewReader(f, info.Size())
	if err != nil {
		return err
	}

	for _, zf := range r.File {
		if err := extractZipFile(fs, zf, dest); err != nil {
			return err
		}
	}

	return nil
}

func extractZipFile(fs fsh.FS, zf *zip.File, dest string) error {
	target := filepath.Join(dest, zf.Name)
	if zf.FileInfo().IsDir() {
		if err := fs.MkdirAll(target, 0755); err != nil {
			return err
		}

		return nil
	}

	if err := fs.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}

	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close() //nolint:errcheck

	out, err := fs.Create(target)
	if err != nil {
		return err
	}
	defer out.Close() //nolint:errcheck

	if _, err := io.Copy(out, rc); err != nil {
		return err
	}

	return nil
}

func extractTar(fs fsh.FS, f afero.File, dest string, wrap func(io.Reader) (io.Reader, error)) error {
	reader, err := wrap(f)
	if err != nil {
		return err
	}

	tr := tar.NewReader(reader)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if err := extractTarFile(fs, tr, dest, hdr); err != nil {
			return err
		}
	}

	return nil
}

func extractTarFile(fs fsh.FS, tr *tar.Reader, dest string, hdr *tar.Header) error {
	target := filepath.Join(dest, hdr.Name)
	switch hdr.Typeflag {
	case tar.TypeDir:
		if err := fs.MkdirAll(target, 0755); err != nil {
			return err
		}
	case tar.TypeReg:
		if err := fs.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		out, err := fs.Create(target)
		if err != nil {
			return err
		}
		defer out.Close() //nolint:errcheck

		if _, err := io.Copy(out, tr); err != nil {
			return err
		}
	}

	return nil
}
