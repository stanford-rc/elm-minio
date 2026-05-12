package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// Patches maps unix filesystem paths to Patcher
var Patches = map[string]Patcher{
	`minio/cmd/utils.go$`:                    PatchMinioGlobals(),
	`minio-pkg/policy/condition/keyname.go$`: PatchPkgJWTClaims(),
}

// Patcher interface implemented by individual implementations to modify the
// minio source code
type Patcher interface {
	Patch(string) (*bytes.Buffer, error)
}

// patchSource applies patcher.Patch to a filepath fpath
func patchSource(fpath string, patcher Patcher, update, backup bool) error {
	// read the original into memory
	forig, err := os.ReadFile(fpath)
	if err != nil {
		return fmt.Errorf("error reading file: %s: %v", fpath, err)
	}

	// run the patcher
	buf, err := patcher.Patch(fpath)
	if err != nil {
		return err
	}

	if !update {
		fmt.Printf("[%s]\n%s", fpath, buf.String())
		return nil
	}

	dir := filepath.Dir(fpath)
	fname := filepath.Base(fpath)

	if backup {
		fbackup := []string{}

		nbackup := 1
		for {
			fbackup = append(fbackup,
				filepath.Join(dir, fmt.Sprintf("%s~%d", fname, nbackup)))
			if _, err := os.Stat(fbackup[len(fbackup)-1]); os.IsNotExist(err) {
				break
			}
			nbackup += 1
		}

		for i := len(fbackup) - 1; i > 0; i-- {
			err := os.Rename(fbackup[i-1], fbackup[i])
			if err != nil {
				return fmt.Errorf("unable to move backup %s to %s: %v",
					fbackup[i-1], fbackup[i], err)
			}
		}

		fh, err := os.Create(fbackup[0])
		if err != nil {
			return fmt.Errorf("error opening backup file: %s: %v", fbackup[0], err)
		}

		defer fh.Close()
		if _, err = fh.Write(forig); err != nil {
			return fmt.Errorf("error writing backup file: %s: %v", fbackup[0], err)
		}
	}

	// write the modified source to a temporary file alongside the original
	fh, err := os.CreateTemp(dir, fname+".")
	if err != nil {
		return fmt.Errorf("error creating temporary file: %v", err)
	}

	// queue cleanup of the temporary file if we fail to create or
	// update
	defer os.Remove(fh.Name())
	defer fh.Close()

	// write the modified source to the temporary file and then
	// rename it to the original fpath filename.
	if _, err = fh.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("error writing replacement file: %s: %v", fh.Name(), err)
	}

	if err = fh.Close(); err != nil {
		return fmt.Errorf("error closing replacement file: %s: %v", fh.Name(), err)
	}

	if err = os.Rename(fh.Name(), fpath); err != nil {
		return fmt.Errorf("error renaming replacement file: %s -> %s: %v", fh.Name(), fpath, err)
	}

	return nil
}
