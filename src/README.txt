In this directory we we can optionally install a copy of the minio or other
source trees.  if you want to override the default of cloning from github,  For
example, let's say you create src/debug/minio/ and src/debug/pkg/ with modified
copies of the github.com/minio/minio and github.com/minio/pkg source trees, and
you linked them into the src directory (the following example is run from the
parent directory of src):

By default this directory won't contain any source directories, and
minio.build.sh will fetch source trees using git clone.  

Note that after minio shut down their OSS project we've cloned their repos:

    github.com/minio/cli            ->  github.com/stanford-rc/minio-cli
    github.com/minio/colorjson      ->  github.com/stanford-rc/minio-colorjson
    github.com/minio/crc64nvme      ->  github.com/stanford-rc/minio-crc64nvme
    github.com/minio/csvparser      ->  github.com/stanford-rc/minio-csvparser
    github.com/minio/dnscache       ->  github.com/stanford-rc/minio-dnscache
    github.com/minio/dperf          ->  github.com/stanford-rc/minio-dperf
    github.com/minio/filepath       ->  github.com/stanford-rc/minio-filepath
    github.com/minio/highwayhash    ->  github.com/stanford-rc/minio-highwayhash
    github.com/minio/kes            ->  github.com/stanford-rc/minio-kes
    github.com/minio/kms-go         ->  github.com/stanford-rc/minio-kms-go
    github.com/minio/madmin-go      ->  github.com/stanford-rc/madmin-go
    github.com/minio/mc             ->  github.com/stanford-rc/minio-mc
    github.com/minio/md5-simd       ->  github.com/stanford-rc/minio-md5-simd
    github.com/minio/minio          ->  github.com/stanford-rc/minio
    github.com/minio/minio-go       ->  github.com/stanford-rc/minio-go
    github.com/minio/mux            ->  github.com/stanford-rc/minio-mux
    github.com/minio/pkg            ->  github.com/stanford-rc/minio-pkg
    github.com/minio/selfupdate     ->  github.com/stanford-rc/minio-selfupdate
    github.com/minio/sha256-simd    ->  github.com/stanford-rc/minio-sha256-simd
    github.com/minio/simdjson-go    ->  github.com/stanford-rc/minio-simdjson-go
    github.com/minio/sio            ->  github.com/stanford-rc/minio-sio
    github.com/minio/websocket      ->  github.com/stanford-rc/minio-websocket
    github.com/minio/xxml           ->  github.com/stanford-rc/minio-xxml
    github.com/minio/zipindex       ->  github.com/stanford-rc/minio-zipindex

    github.com/georgmangold/console ->  github.com/stanford-rc/minio-console


Dockerfile copies src/* into its build context, and minio.build.sh  will use
them instead of git cloning the projects, meaning your modified minio and pkg
source will be used instead.  Note that if you provide a source tree it is
expected that it'll be at the correct version of the project.
