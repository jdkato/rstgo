package rstgo

import (
	"fmt"
	"image"
	_ "image/gif"  // register decoders used by readImageSize
	_ "image/jpeg" //
	_ "image/png"  //
	"os"
)

// readImageSize returns the pixel size of an image file, standing in for
// the Python Imaging Library that Docutils uses (PNG, JPEG and GIF only).
// errStr mimics the text of the Python exception on failure.
func readImageSize(path string) (w, h int, errStr string) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, pyOSError(err, path)
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.IsDir() {
		return 0, 0, fmt.Sprintf("[Errno 21] Is a directory: %s", pyStrRepr(path))
	}
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, fmt.Sprintf("cannot identify image file %s", pyStrRepr(path))
	}
	return cfg.Width, cfg.Height, ""
}

// pyOSError formats an error like str(OSError) in Python.
func pyOSError(err error, path string) string {
	switch {
	case os.IsNotExist(err):
		return fmt.Sprintf("[Errno 2] No such file or directory: %s", pyStrRepr(path))
	case os.IsPermission(err):
		return fmt.Sprintf("[Errno 13] Permission denied: %s", pyStrRepr(path))
	}
	if fi, e2 := os.Stat(path); e2 == nil && fi.IsDir() {
		return fmt.Sprintf("[Errno 21] Is a directory: %s", pyStrRepr(path))
	}
	return err.Error()
}
