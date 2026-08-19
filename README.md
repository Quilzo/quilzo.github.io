<img src="images/mark.svg" alt="" width="72" height="72">

# quilzo.github.io

The Quilzo manual. **[quilzo.github.io](https://quilzo.github.io)**

Quilzo is a content management system where stored content is immutable,
publishing moves a pointer, and the template language cannot execute anything.
The program lives at [Quilzo/Quilzo](https://github.com/Quilzo/Quilzo); this is
its documentation.

## Why the manual is its own repository

It used to be compiled into the binary — about 1,800 lines of Go describing
every screen, served at `/docs`, with the screenshots embedded in the
executable. That made documentation a release artefact. A wrong sentence waited
for a build and a tag before anybody could read the right one, and a screenshot
went stale the moment a screen changed.

Documentation is corrected far more often than software is released, so it is
published on its own now and can be fixed the day somebody notices.

The cost of splitting it out is that the application can no longer prove its
Help links land somewhere. That is what `sections.txt` is for.

## The contract with the application

Every screen in the Quilzo admin has a Help link in the same place, pointing at
the section for the screen you are looking at — `#types` from the Types screen,
`#logging` from the audit log. A link that lands on a heading somebody renamed
is worse than no link at all: the person following it concludes the feature was
removed, which is the belief documentation exists to correct.

Two halves keep that honest, one in each repository:

| Where | What it refuses |
|---|---|
| `sections.txt` + `check-anchors.sh` here | a build where `index.html` has stopped carrying an anchor the app links to |
| `docSections` in `internal/admin/nav.go` there | a screen pointing at an anchor this manual does not publish |

Neither half proves the other. Each fails loudly on its own side, and the
failure a reader actually suffers — Help landing on nothing — needs both to be
wrong at once.

**Renaming a section means changing it in both repositories.** That is the real
cost of splitting the manual out, and it is cheaper than a Help link that 404s.

```sh
./check-anchors.sh    # run before pushing a section rename
```

## What's here

```
index.html        the manual — one page, ~30 sections
style.css         its styles
app.js            table-of-contents highlighting and the mobile contents toggle
images/           the 8 screenshots
sections.txt      the anchors the application links to (see above)
check-anchors.sh  verifies index.html still carries every one of them
.nojekyll         serve these files as-is, without Jekyll
```

No build step and no framework. Edit `index.html`, push to `main`, and GitHub
Pages serves it.

## Editing

- **Fixing wording** — edit the section in `index.html` and push.
- **Adding a section** — give it an `id`, add it to the table of contents in the
  sidebar, and add it to `sections.txt` only if a screen in the app links to it.
- **Renaming a section** — change the `id` here *and* `docSections` in
  `internal/admin/nav.go` in the main repository, in the same change if you can.
- **Replacing a screenshot** — same filename in `images/`, so nothing else moves.

CI checks three things on every push: that every anchor in `sections.txt`
exists, that no in-page link points at a missing section, and that every
referenced image is actually in the repository.

## Licence

The software is AGPL-3.0-or-later. This documentation describes it and carries
the same licence.
