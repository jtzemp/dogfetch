#!/bin/sh
set -eu

if [ "$#" -lt 1 ]; then
    echo "usage: $0 <version>" >&2
    exit 1
fi

version="${1#v}"
plugin_file=".claude-plugin/plugin.json"

[ -f "$plugin_file" ] || {
    echo "missing plugin manifest: $plugin_file" >&2
    exit 1
}

jq --arg version "$version" '.version = $version' "$plugin_file" > "$plugin_file.tmp"
mv "$plugin_file.tmp" "$plugin_file"

echo "Updated $plugin_file to version $version"
