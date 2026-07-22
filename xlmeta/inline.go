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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/minio/highwayhash"
	"github.com/tinylib/msgp/msgp"
)

const inlineDataVer = 1

// magicHighwayHash256Key is the fixed key MinIO uses for HighwayHash-based
// bitrot verification of inline data blocks.
const magicHighwayHash256Key = "\x4b\xe7\x34\xfa\x8e\x23\x8a\xcd" +
	"\x26\x3e\x83\xe6\xbb\x96\x85\x52" +
	"\x04\x0f\x93\x5d\xa3\x9f\x44\x14" +
	"\x97\xe0\x9d\x13\x22\xde\x36\xa0"

// InlineData wraps the raw inline-data section of an xl.meta blob. The first
// byte is a version indicator; the remainder is a msgpack map of
// name -> [32-byte HighwayHash][payload].
type InlineData []byte

// afterVersion returns the payload following the one-byte version prefix.
func (x InlineData) afterVersion() []byte {
	if len(x) == 0 {
		return x
	}
	return x[1:]
}

// versionOK reports whether the inline-data version byte is supported. An
// empty slice (no inline data) is considered valid.
func (x InlineData) versionOK() bool {
	if len(x) == 0 {
		return true
	}
	return x[0] > 0 && x[0] <= inlineDataVer
}

// Entry is one named inline data block after decoding and bitrot verification.
type Entry struct {
	// Name is the map key: a version UUID string, "null", or a "part.N" key.
	Name string
	// Data is the payload with the 32-byte bitrot hash stripped. It is nil
	// when the block is too short to contain a hash + data.
	Data []byte
	// BitrotOK is true when the stored HighwayHash matches Data.
	BitrotOK bool
}

// Entries decodes every inline data block, verifying bitrot for each.
func (x InlineData) Entries() ([]Entry, error) {
	var out []Entry
	err := x.ForEach(func(name string, val []byte) {
		e := Entry{Name: name}
		if len(val) >= 32 {
			e.Data = val[32:]
			e.BitrotOK = verifyBitrot(val[:32], e.Data)
		}
		out = append(out, e)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ForEach iterates over the named inline data blocks, calling fn with the key
// and the raw bytes (including the 32-byte hash prefix).
func (x InlineData) ForEach(fn func(name string, val []byte)) error {
	if len(x) == 0 {
		return nil
	}
	if !x.versionOK() {
		return errors.New("xlMetaInlineData: unknown version")
	}

	sz, buf, err := msgp.ReadMapHeaderBytes(x.afterVersion())
	if err != nil {
		return err
	}
	for i := range sz {
		var key, val []byte
		key, buf, err = msgp.ReadMapKeyZC(buf)
		if err != nil {
			return err
		}
		if len(key) == 0 {
			return fmt.Errorf("xlMetaInlineData: key %d is length 0", i)
		}
		val, buf, err = msgp.ReadBytesZC(buf)
		if err != nil {
			return err
		}
		fn(string(key), val)
	}
	return nil
}

// JSON renders the inline data section the way MinIO's xl-meta --data flag
// does: a JSON object of name -> {bytes, bitrot_valid, [data_string],
// data_base64}. When includeValues is false the payload fields are omitted.
func (x InlineData) JSON(includeValues bool) ([]byte, error) {
	if len(x) == 0 {
		return []byte("{}"), nil
	}
	if !x.versionOK() {
		return nil, errors.New("xlMetaInlineData: unknown version")
	}

	res := []byte("{")
	first := true
	err := x.ForEach(func(key string, val []byte) {
		if !first {
			res = append(res, ',')
		}
		first = false
		res = append(res, []byte(fmt.Sprintf(`"%s": {"bytes": %d`, key, len(val)))...)
		if len(val) >= 32 {
			data := val[32:]
			if verifyBitrot(val[:32], data) {
				res = append(res, []byte(`, "bitrot_valid": true`)...)
			} else {
				res = append(res, []byte(`, "bitrot_valid": false`)...)
			}
			if includeValues {
				if utf8.Valid(data) {
					if b, err := json.Marshal(string(data)); err == nil {
						res = append(res, []byte(`, "data_string": `)...)
						res = append(res, b...)
					}
				}
				res = append(res, []byte(`, "data_base64": "`)...)
				res = append(res, []byte(base64.StdEncoding.EncodeToString(data))...)
				res = append(res, '"')
			}
		}
		res = append(res, '}')
	})
	if err != nil {
		return nil, err
	}
	res = append(res, '}')
	return res, nil
}

// verifyBitrot reports whether the HighwayHash of data equals want.
func verifyBitrot(want, data []byte) bool {
	hh, _ := highwayhash.New([]byte(magicHighwayHash256Key))
	hh.Write(data)
	return bytes.Equal(want, hh.Sum(nil))
}
