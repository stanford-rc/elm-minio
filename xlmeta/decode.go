// Copyright (c) 2015-2021 MinIO, Inc.
// Copyright (c) 2026 The Board of Trustees of the Leland Stanford Junior University
//
// This file is derived from MinIO's docs/debugging/xl-meta tool and is
// distributed under the GNU Affero General Public License v3.0. See the
// LICENSE file at the root of this repository.
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package xlmeta

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/tinylib/msgp/msgp"
)

// FileMeta is the decoded content of a single xl.meta file.
type FileMeta struct {
	// Path is the file the metadata was read from (empty for Decode).
	Path string
	// Minor is the XL2 container minor version (0, 1, 2, or 3).
	Minor uint16
	// Versions holds the typed per-version metadata. It is populated for the
	// current v3 format; legacy formats leave it empty (see JSON).
	Versions []Version
	// Inline is the raw inline-data section (may be empty).
	Inline InlineData
	// JSON is a rendering equivalent to MinIO's xl-meta tool output for this
	// file (the "{...}" value, before the tool wraps it under a filename key).
	// It is retained as a validation oracle and for debugging.
	JSON []byte
}

// Version is one object version within an xl.meta file.
type Version struct {
	Idx    int
	Header Header
	// Object is the decoded V2 object metadata, or nil when the version is
	// not an inline/erasure object (e.g. a delete marker).
	Object *ObjectV2
}

// ObjectV2 mirrors MinIO's xlMetaV2Object fields as decoded from the msgpack
// metadata. Field names match the JSON keys MinIO's tool emits.
type ObjectV2 struct {
	ID         string            // version UUID (base64 or hex form)
	DDir       string            // data directory UUID
	EcAlgo     int               // erasure algorithm
	EcM        int               // data blocks
	EcN        int               // parity blocks
	EcBSize    int               // erasure block size
	EcIndex    int               // 1-based shard index for this disk
	EcDist     []int             // shard distribution across the disk set
	CSumAlgo   int               // bitrot checksum algorithm
	PartNums   []int             // part numbers
	PartETags  []string          // per-part ETags (usually empty)
	PartSizes  []int64           // per-part sizes
	PartASizes []int64           // per-part actual (compressed) sizes
	Size       int64             // total object size
	MTime      int64             // modification time (unix nanos)
	MetaSys    map[string][]byte // system metadata (e.g. x-minio-internal-*)
	MetaUsr    map[string]string // user metadata (e.g. etag)
}

// metaEnvelope is the JSON shape of a decoded version's metadata blob.
type metaEnvelope struct {
	Type  int
	V2Obj *ObjectV2
}

// DecodeFile reads and decodes the xl.meta file at path.
func DecodeFile(path string) (*FileMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fm, err := Decode(f, path)
	if err != nil {
		return nil, err
	}
	fm.Path = path
	return fm, nil
}

// Decode reads an xl.meta blob from r and decodes it. name is used only for
// error context and may be empty.
func Decode(r io.Reader, name string) (*FileMeta, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b, _, minor, err := CheckXL2V1(b)
	if err != nil {
		return nil, err
	}

	fm := &FileMeta{Path: name, Minor: minor}
	switch minor {
	case 0:
		err = decodeV0(fm, b)
	case 1, 2:
		err = decodeV1V2(fm, b)
	case 3:
		err = decodeV3(fm, b)
	default:
		return nil, fmt.Errorf("unknown metadata version %d", minor)
	}
	if err != nil {
		return nil, err
	}
	return fm, nil
}

// decodeV0 handles the oldest single-blob format.
func decodeV0(fm *FileMeta, b []byte) error {
	var buf bytes.Buffer
	if _, err := msgp.CopyToJSON(&buf, bytes.NewReader(b)); err != nil {
		return err
	}
	fm.JSON = buf.Bytes()
	return nil
}

// decodeV1V2 handles the single-version formats that carry inline data and an
// optional trailing CRC.
func decodeV1V2(fm *FileMeta, b []byte) error {
	v, rest, err := msgp.ReadBytesZC(b)
	if err != nil {
		return err
	}
	if _, nbuf, e := msgp.ReadUint32Bytes(rest); e == nil {
		rest = nbuf // metadata CRC (v2), skip if present
	}
	var buf bytes.Buffer
	if _, err = msgp.CopyToJSON(&buf, bytes.NewReader(v)); err != nil {
		return err
	}
	fm.JSON = buf.Bytes()
	fm.Inline = InlineData(rest)
	return nil
}

// decodeV3 handles the current header + per-version format.
func decodeV3(fm *FileMeta, b []byte) error {
	v, rest, err := msgp.ReadBytesZC(b)
	if err != nil {
		return err
	}
	if _, nbuf, e := msgp.ReadUint32Bytes(rest); e == nil {
		rest = nbuf // metadata CRC, skip if present
	}
	fm.Inline = InlineData(rest)

	hdr, v, err := decodeXLHeaders(v)
	if err != nil {
		return err
	}

	// jsonVersion mirrors the upstream tool's per-version JSON shape.
	type jsonVersion struct {
		Idx      int
		Header   json.RawMessage
		Metadata json.RawMessage
	}
	jsonVersions := make([]jsonVersion, hdr.versions)
	fm.Versions = make([]Version, hdr.versions)

	err = decodeVersions(v, hdr.versions, func(idx int, hdrBytes, meta []byte) error {
		var header Header
		if _, err := header.UnmarshalMsg(hdrBytes, hdr.headerVer); err != nil {
			return err
		}
		headerJSON, err := header.MarshalJSON()
		if err != nil {
			return err
		}
		var metaBuf bytes.Buffer
		if _, err := msgp.UnmarshalAsJSON(&metaBuf, meta); err != nil {
			return err
		}

		var env metaEnvelope
		if err := json.Unmarshal(metaBuf.Bytes(), &env); err != nil {
			return fmt.Errorf("decoding version %d metadata: %w", idx, err)
		}

		fm.Versions[idx] = Version{Idx: idx, Header: header, Object: env.V2Obj}
		jsonVersions[idx] = jsonVersion{
			Idx:      idx,
			Header:   headerJSON,
			Metadata: metaBuf.Bytes(),
		}
		return nil
	})
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(struct{ Versions []jsonVersion }{Versions: jsonVersions}); err != nil {
		return err
	}
	fm.JSON = buf.Bytes()
	return nil
}
