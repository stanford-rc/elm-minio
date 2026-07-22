// Copyright (c) 2026 The Board of Trustees of the Leland Stanford Junior University
//
// Distributed under the GNU Affero General Public License v3.0. See the
// LICENSE file at the root of this repository.
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package xlmeta_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stanford-rc/elm-minio/xlmeta"
)

// TestCompatAgainstBinary decodes every xl.meta file under $XLMETA_RIG with
// the library and asserts the result matches MinIO's xl-meta tool ($XLMETA_BIN)
// byte-for-byte after JSON normalization, both for the default metadata output
// and the --data inline output.
//
// It is skipped unless both env vars are set:
//
//	XLMETA_BIN=/path/to/xl-meta XLMETA_RIG=/path/to/rig go test -run Compat -v ./...
func TestCompatAgainstBinary(t *testing.T) {
	bin := os.Getenv("XLMETA_BIN")
	rig := os.Getenv("XLMETA_RIG")
	if bin == "" || rig == "" {
		t.Skip("set XLMETA_BIN and XLMETA_RIG to run the live-MinIO comparison")
	}

	var files []string
	err := filepath.Walk(rig, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "xl.meta" {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", rig, err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatalf("no xl.meta files found under %s", rig)
	}
	t.Logf("comparing %d xl.meta files", len(files))

	for _, f := range files {
		rel, _ := filepath.Rel(rig, f)

		t.Run("meta/"+rel, func(t *testing.T) {
			fm, err := xlmeta.DecodeFile(f)
			if err != nil {
				t.Fatalf("DecodeFile: %v", err)
			}
			got := normalize(t, fm.JSON)
			want := normalize(t, binValue(t, bin, f))
			if got != want {
				t.Errorf("metadata mismatch\n--- library ---\n%s\n--- binary ---\n%s", got, want)
			}
		})

		t.Run("data/"+rel, func(t *testing.T) {
			fm, err := xlmeta.DecodeFile(f)
			if err != nil {
				t.Fatalf("DecodeFile: %v", err)
			}
			libInline, err := fm.Inline.JSON(true)
			if err != nil {
				t.Fatalf("Inline.JSON: %v", err)
			}
			got := normalize(t, libInline)
			want := normalize(t, binValue(t, bin, f, "--data"))
			if got != want {
				t.Errorf("inline-data mismatch\n--- library ---\n%s\n--- binary ---\n%s", got, want)
			}
		})
	}
}

// binValue runs the xl-meta binary in --ndjson mode over a single file and
// returns the decoded value for that file. --ndjson is required because the
// tool's single-file (non-ndjson) path brace-trims its output, which corrupts
// the --data inline object (it ends in "}}", so the trim eats a closing brace).
// In --ndjson mode the tool emits a clean {"<file>": <value>} object.
func binValue(t *testing.T, bin string, file string, extra ...string) []byte {
	t.Helper()
	args := append(append([]string{"--ndjson"}, extra...), file)
	cmd := exec.Command(bin, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("xl-meta %v: %v (stderr: %s)", args, err, errb.String())
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("parsing xl-meta %v output: %v\noutput: %s", args, err, out.Bytes())
	}
	for _, v := range m {
		return v
	}
	t.Fatalf("xl-meta %v produced no entries: %s", args, out.Bytes())
	return nil
}

// normalize parses JSON preserving integer precision and re-marshals it so
// key ordering and whitespace do not affect equality.
func normalize(t *testing.T, b []byte) string {
	t.Helper()
	var v any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("normalize: %v\ninput: %s", err, b)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("normalize marshal: %v", err)
	}
	return string(out)
}
