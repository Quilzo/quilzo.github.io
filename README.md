<img src="images/mark.svg" alt="" width="72" height="72">

# quilzo.github.io

The manual for [Quilzo](https://github.com/Quilzo/Quilzo), the self-hosted
control plane for AI agents: **[quilzo.github.io](https://quilzo.github.io)**.

A guide per page, each with steps taken in a real store and screenshots of the
screens it describes, in the admin's own Material 3 Expressive design. Every
screen in the Quilzo admin has a Help link to its guide.

## How it is built

```
src/pages/*.html   one guide each: a header comment, then the body
src/nav.txt        the menu, in order
src/layout.html    the frame around every page
src/legacy.txt     the old one-page manual's anchors, and where each moved
assets/            the stylesheet, the one script, the font and the icons
images/            screenshots, as WebP, one folder per guide
gen/               the generator: Go, standard library only
sections.txt       every guide the Quilzo admin links to
```

```sh
go run ./gen           # build every page, search.json, sitemap.xml, robots.txt and llms.txt
go run ./gen -check    # what CI runs: fail if anything differs from src/,
                       # a link lands nowhere, or a picture is missing
```

Each page's header sets its heading, its title and description for search
results (the generator refuses a title over about 60 characters or a
description outside 70 to 165), the admin screens it is the Help for, and the
date it last changed. Pages carry JSON-LD (TechArticle and breadcrumbs; the
home page SoftwareApplication and WebSite), Open Graph cards and a canonical
URL.

## The contract with the application

A Help link is `https://quilzo.github.io/SLUG/`, and `docSections` in
`internal/admin/nav.go` in the Quilzo repository lists the slugs. `sections.txt`
lists the same, and the build fails if one is not a page. Renaming a guide means
changing it in both repositories. A Help link from a Quilzo built before the
manual had pages lands on `/#anchor`; the home page sends it to the page in
`src/legacy.txt`.

## Licence

The software is `AGPL-3.0-or-later OR LicenseRef-Quilzo-Commercial`, at the
user's choice; see [LICENSING.md](https://github.com/Quilzo/Quilzo/blob/main/LICENSING.md).
This manual describes it and carries the same terms. The interface font is
derived from Google Sans Flex (SIL Open Font License 1.1, in `assets/fonts`), and
the icons are Material Symbols (Apache-2.0, in `assets/icons`).
