// Copyright (c) 2015-2021 MinIO, Inc.
// Copyright (c) 2026 The Board of Trustees of the Leland Stanford Junior University
//
// This file is derived from MinIO's docs/debugging/xl-meta tool and is
// distributed under the GNU Affero General Public License v3.0. See the
// LICENSE file at the root of this repository.
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package xlmeta decodes MinIO "xl.meta" object-metadata files natively,
// without shelling out to MinIO's xl-meta debug tool.
//
// The decode logic is ported from MinIO's docs/debugging/xl-meta tool
// (AGPLv3, MinIO Inc.). It reads the XL2 binary container, the per-version
// headers, the msgpack-encoded V2 object metadata, and the inline data
// section (with HighwayHash bitrot verification), exposing the results both
// as typed Go structs and as a JSON rendering that matches the upstream
// tool's output byte-for-byte (used as a validation oracle).
package xlmeta

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// xlHeader is the 4-byte magic that identifies an XL v2 metadata file.
var xlHeader = [4]byte{'X', 'L', '2', ' '}

const (
	// xlVersionMajor indicates breaking format changes. A file whose major
	// version exceeds this cannot be read.
	xlVersionMajor = 1

	// xlVersionMinor is informational; readers accept any minor version they
	// have a decode path for (see Decode).
	xlVersionMinor = 1
)

// CheckXL2V1 validates the 8-byte XL v2 header and returns the payload after
// it, along with the major and minor format versions.
func CheckXL2V1(buf []byte) (payload []byte, major, minor uint16, err error) {
	if len(buf) <= 8 {
		return nil, 0, 0, fmt.Errorf("xlMeta: no data")
	}
	if !bytes.Equal(buf[:4], xlHeader[:]) {
		return nil, 0, 0, fmt.Errorf("xlMeta: unknown XLv2 header, expected %v, got %v",
			xlHeader[:4], buf[:4])
	}

	// The earliest format used the ASCII string "1   " for the version field.
	if bytes.Equal(buf[4:8], []byte("1   ")) {
		major, minor = 1, 0
	} else {
		major = binary.LittleEndian.Uint16(buf[4:6])
		minor = binary.LittleEndian.Uint16(buf[6:8])
	}

	if major > xlVersionMajor {
		return buf[8:], major, minor,
			fmt.Errorf("xlMeta: unknown major version %d found", major)
	}
	return buf[8:], major, minor, nil
}
