# xlmeta

A native Go decoder for MinIO `xl.meta` object-metadata files.

## Status: reference implementation

This package is **not currently linked into any active Stanford RC code**. It
lives here, inside the AGPL-licensed `elm-minio` repository, deliberately: it
is a port of MinIO's `docs/debugging/xl-meta` tool (Copyright MinIO, Inc.;
AGPLv3), so keeping it in this repo keeps that derived code within its existing
license boundary rather than pulling AGPL obligations into other Stanford RC
codebases.

The original motivation was to let `read-elm-object` (in the `elm` repo) decode
`xl.meta` in-process instead of shelling out to the external `xl-meta` binary.
That integration is **not** done and should not be done without first settling
the licensing question — linking this into a shipped elm binary is a stronger
copyleft trigger than the current arm's-length exec of the debug tool.

## What it does

`Decode` / `DecodeFile` parse the XL2 container, the per-version headers, the
msgpack V2 object metadata, and the inline-data section (with HighwayHash
bitrot verification), exposing results as:

- typed Go structs — `FileMeta` → `[]Version`, each with a typed `Header`
  (plus `Flags` helpers) and `*ObjectV2` (EcDist, EcBSize, CSumAlgo, PartSizes,
  MetaSys, MetaUsr, …);
- `FileMeta.Inline.Entries()` for bitrot-verified inline shard data;
- `FileMeta.JSON`, a rendering equivalent to the upstream tool's output, used
  as a validation oracle.

Only the decode path is ported; the upstream tool's shard combine/reconstruct
logic is omitted.

## Validation

`compat_test.go` decodes every `xl.meta` under `$XLMETA_RIG` and asserts the
result matches the upstream `xl-meta` binary (`$XLMETA_BIN`) byte-for-byte
after JSON normalization, for both the default metadata output and `--data`
inline output. It is skipped unless both env vars are set:

```sh
XLMETA_BIN=/path/to/xl-meta \
XLMETA_RIG=/path/to/minio-rig \
go test -run Compat -v ./...
```

It was validated against a live 4-drive erasure rig (inline, single-part-EC,
and multipart-EC objects, plus MinIO's internal `.minio.sys` files): 56 files
× {metadata, --data} = 112 comparisons, all matching.
