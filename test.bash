#!/bin/bash
# This is free and unencumbered software released into the public domain.
# For more information, please refer to <https://unlicense.org>

set -e

test=makefat-test

cat <<EOF > "$test.go"
package main
import "fmt"
func main() { fmt.Println("Fat binary OK") }
EOF

GOOS=darwin GOARCH=amd64 go build -o "$test-amd64" "$test.go"
GOOS=darwin GOARCH=arm64 go build -o "$test-arm64" "$test.go"

if ./makefat "$test" "$test-amd64" "$test-arm64"; then
   if [ "$(go env GOOS)" = darwin ]; then
      "./$test" || true
      rm "$test"
   else
      echo "$test built; please try it on MacOS"
   fi
fi

rm "$test.go" "$test-amd64" "$test-arm64"
