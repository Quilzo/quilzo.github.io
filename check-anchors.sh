#!/bin/sh
# Every anchor the application links to must exist in the manual.
#
# The admin's Help link is the most-followed link in the product and the one
# nobody notices breaking: it renders fine, it looks like a link, and it lands
# on nothing. Renaming a section here is what breaks it, and this is the only
# place that can catch it.
set -eu

missing=0
while IFS= read -r anchor; do
    case "$anchor" in
        ''|\#*) continue ;;
    esac
    if ! grep -q "id=\"$anchor\"" index.html; then
        echo "missing: #$anchor is linked from the app and no element in index.html has that id"
        missing=$((missing + 1))
    fi
done < sections.txt

if [ "$missing" -gt 0 ]; then
    echo
    echo "$missing section(s) the application links to are not in this manual."
    echo "Either restore the id, or remove the anchor here AND from docSections"
    echo "in internal/admin/nav.go in the Quilzo repository."
    exit 1
fi

echo "all sections present"
