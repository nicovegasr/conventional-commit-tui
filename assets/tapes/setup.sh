#!/usr/bin/env bash
# Prepares the demo repository the README tapes record: a fresh clone of this
# repository with a few uncommitted edits, a nib binary on PATH and its index.
# Usage (from the repository root): source assets/tapes/setup.sh
set -euo pipefail
root=$(git rev-parse --show-toplevel)
demo=${TMPDIR:-/tmp}/nib-demo
rm -rf "$demo"
git clone --quiet "$root" "$demo"
(cd "$root" && go build -o "$demo/.bin/nib" .)
cd "$demo"
echo "// related files demo" >> internal/ui/model.go
echo "// related files demo" >> internal/ui/model_test.go
echo "// related files demo" >> main.go
printf '# notes\n' > notes.md
printf '.bin/\nnotes.md\n' >> .git/info/exclude
./.bin/nib index > /dev/null
export PATH="$demo/.bin:$PATH"
set +euo pipefail
clear
