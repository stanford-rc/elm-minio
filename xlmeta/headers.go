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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tinylib/msgp/msgp"
)

const (
	xlHeaderVersion = 3
	xlMetaVersion   = 3
)

// Flags carries the per-version state bits from the XL header.
type Flags uint8

const (
	flagFreeVersion Flags = 1 << iota
	flagUsesDataDir
	flagInlineData
)

// FreeVersion reports whether this version is a free (tombstone) version.
func (f Flags) FreeVersion() bool { return f&flagFreeVersion != 0 }

// UsesDataDir reports whether the version's data lives in its data directory.
func (f Flags) UsesDataDir() bool { return f&flagUsesDataDir != 0 }

// InlineData reports whether the version's data is stored inline. A false
// result only means inline data is unlikely, not guaranteed absent.
func (f Flags) InlineData() bool { return f&flagInlineData != 0 }

// xlHeaders is the decoded preamble that precedes the version entries in a
// v3 XL metadata blob.
type xlHeaders struct {
	versions  int
	headerVer uint
	metaVer   uint
}

// decodeXLHeaders reads the header version, meta version, and version count,
// returning the remaining bytes.
func decodeXLHeaders(buf []byte) (x xlHeaders, rest []byte, err error) {
	x.headerVer, buf, err = msgp.ReadUintBytes(buf)
	if err != nil {
		return x, buf, err
	}
	x.metaVer, buf, err = msgp.ReadUintBytes(buf)
	if err != nil {
		return x, buf, err
	}
	if x.headerVer > xlHeaderVersion {
		return x, buf, fmt.Errorf("decodeXLHeaders: unknown xl header version %d", x.headerVer)
	}
	if x.metaVer > xlMetaVersion {
		return x, buf, fmt.Errorf("decodeXLHeaders: unknown xl meta version %d", x.metaVer)
	}
	x.versions, buf, err = msgp.ReadIntBytes(buf)
	if err != nil {
		return x, buf, err
	}
	if x.versions < 0 {
		return x, buf, fmt.Errorf("decodeXLHeaders: negative version count %d", x.versions)
	}
	return x, buf, nil
}

// decodeVersions reads count (header, meta) byte pairs and invokes fn for each
// in order (newest first).
func decodeVersions(buf []byte, count int, fn func(idx int, hdr, meta []byte) error) error {
	var (
		tHdr, tMeta []byte
		err         error
	)
	for i := range count {
		tHdr, buf, err = msgp.ReadBytesZC(buf)
		if err != nil {
			return err
		}
		tMeta, buf, err = msgp.ReadBytesZC(buf)
		if err != nil {
			return err
		}
		if err = fn(i, tHdr, tMeta); err != nil {
			return err
		}
	}
	return nil
}

// Header is the binary-packed per-version header. EcN and EcM are only present
// for header version >= 3 (they are 0 otherwise).
type Header struct {
	VersionID [16]byte
	ModTime   int64
	Signature [4]byte
	Type      uint8
	Flags     Flags
	EcN, EcM  uint8
}

// UnmarshalMsg decodes a msgpack array into the header. hdrVer controls
// whether the EcN/EcM fields are expected (version > 2).
func (z *Header) UnmarshalMsg(bts []byte, hdrVer uint) ([]byte, error) {
	count, bts, err := msgp.ReadArrayHeaderBytes(bts)
	if err != nil {
		return nil, msgp.WrapError(err)
	}

	want := uint32(5)
	if hdrVer > 2 {
		want += 2
	}
	if count != want {
		return nil, msgp.ArrayError{Wanted: want, Got: count}
	}

	bts, err = msgp.ReadExactBytes(bts, z.VersionID[:])
	if err != nil {
		return nil, msgp.WrapError(err, "VersionID")
	}
	z.ModTime, bts, err = msgp.ReadInt64Bytes(bts)
	if err != nil {
		return nil, msgp.WrapError(err, "ModTime")
	}
	bts, err = msgp.ReadExactBytes(bts, z.Signature[:])
	if err != nil {
		return nil, msgp.WrapError(err, "Signature")
	}
	var t uint8
	t, bts, err = msgp.ReadUint8Bytes(bts)
	if err != nil {
		return nil, msgp.WrapError(err, "Type")
	}
	z.Type = t
	var fl uint8
	fl, bts, err = msgp.ReadUint8Bytes(bts)
	if err != nil {
		return nil, msgp.WrapError(err, "Flags")
	}
	z.Flags = Flags(fl)

	if hdrVer > 2 {
		z.EcN, bts, err = msgp.ReadUint8Bytes(bts)
		if err != nil {
			return nil, msgp.WrapError(err, "EcN")
		}
		z.EcM, bts, err = msgp.ReadUint8Bytes(bts)
		if err != nil {
			return nil, msgp.WrapError(err, "EcM")
		}
	}
	return bts, nil
}

// MarshalJSON renders the header the same way MinIO's xl-meta tool does:
// hex-encoded IDs, an RFC3339 timestamp, and field order VersionID, ModTime,
// Signature, Type, Flags, EcM, EcN.
func (z Header) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		VersionID string
		ModTime   time.Time
		Signature string
		Type      uint8
		Flags     uint8
		EcM, EcN  uint8
	}{
		VersionID: hex.EncodeToString(z.VersionID[:]),
		ModTime:   time.Unix(0, z.ModTime),
		Signature: hex.EncodeToString(z.Signature[:]),
		Type:      z.Type,
		Flags:     uint8(z.Flags),
		EcM:       z.EcM,
		EcN:       z.EcN,
	})
}
