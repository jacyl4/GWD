#!/usr/bin/env bash

set -euo pipefail

ARCHIVE_DIR="archive"

if [[ ! -d "${ARCHIVE_DIR}" ]]; then
  exit 0
fi

find "${ARCHIVE_DIR}" -type f ! -name '*.sha256sum' -print0 | while IFS= read -r -d '' file; do
  sha256sum "${file}" | awk '{print $1}' > "${file}.sha256sum"
done
