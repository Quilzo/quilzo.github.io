// Command gen builds the Quilzo manual from src/ into the site's pages.
//
// The pages are plain HTML, written by hand in src/pages, one file each. This
// program puts the same frame around every one of them: the bar, the menu, the
// list of sections down the side, the way to the previous and next guide. It
// also gives every picture its size, so a page does not jump as it loads, and
// writes the index the search box reads.
//
//	go run ./gen          build the site
//	go run ./gen -check   build it in memory and fail if what is committed
//	                      differs, a link lands nowhere, or a picture is missing
//
// Nothing but the standard library, as with Quilzo itself.
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"html/template"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Site is where the manual is published.
const Site = "https://quilzo.github.io"

// Page is one guide.
type Page struct {
	Slug    string // "agents", or "spec/agent-governance/v1"
	Title   string // the heading on the page
	SEO     string // the window's title, which is what a search result shows
	Desc    string // the meta description, which is the snippet under it
	Updated string // when the page last changed in substance, 2006-01-02
	Image   string // the picture a shared link shows
	Short   string // the menu's label
	Lead    string
	Icon    string
	Screens []string // the admin screens whose Help link lands here
	Group   string
	Hidden  bool // reachable, and not in the menu
	Body    template.HTML
	LD      template.JS // what a search engine reads about the page
	TOC     []Heading
	Prev    *Page
	Next    *Page
	URL     string
	Source  string
}

// Heading is a section of a page.
type Heading struct {
	ID, Text string
}

// Group is a heading in the menu.
type Group struct {
	Name  string
	Icon  string
	Pages []*Page
}

// ID is the group's anchor on the home page.
func (g *Group) ID() string { return slugify(g.Name) }

var (
	check = flag.Bool("check", false, "fail if the committed site differs from what src/ builds")
	draft = flag.Bool("draft", false, "report broken links and carry on, while pages are being written")
	root  = flag.String("root", ".", "the repository")
)

func main() {
	flag.Parse()
	out, err := build(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
	if *check {
		var stale []string
		for name, body := range out {
			have, err := os.ReadFile(filepath.Join(*root, name))
			if err != nil || !bytes.Equal(have, body) {
				stale = append(stale, name)
			}
		}
		sort.Strings(stale)
		if len(stale) > 0 {
			fmt.Fprintf(os.Stderr, "gen: %d files are not what src/ builds; run `go run ./gen` and commit:\n  %s\n",
				len(stale), strings.Join(stale, "\n  "))
			os.Exit(1)
		}
		fmt.Printf("%d files match src/; every link and picture resolves\n", len(out))
		return
	}
	names := make([]string, 0, len(out))
	for name := range out {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := filepath.Join(*root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "gen:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(p, out[name], 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "gen:", err)
			os.Exit(1)
		}
	}
	fmt.Printf("wrote %d files\n", len(out))
}

// build reads src/ and returns every file the site is made of, by path.
func build(dir string) (map[string][]byte, error) {
	groups, pages, err := readNav(dir)
	if err != nil {
		return nil, err
	}
	icons, err := readIcons(filepath.Join(dir, "assets", "icons"))
	if err != nil {
		return nil, err
	}
	tpl, err := template.New("layout.html").Funcs(template.FuncMap{
		"icon": func(name string) template.HTML { return icons.svg(name) },
	}).ParseFiles(filepath.Join(dir, "src", "layout.html"))
	if err != nil {
		return nil, err
	}
	bySlug := map[string]*Page{}
	for _, p := range pages {
		bySlug[p.Slug] = p
	}
	// Every page, menu or not, and the order the menu reads in.
	var listed []*Page
	for _, g := range groups {
		listed = append(listed, g.Pages...)
	}
	for i, p := range listed {
		if i > 0 {
			p.Prev = listed[i-1]
		}
		if i+1 < len(listed) {
			p.Next = listed[i+1]
		}
	}
	for _, p := range pages {
		if err := p.render(dir, icons); err != nil {
			return nil, fmt.Errorf("%s: %w", p.Source, err)
		}
	}
	legacy, err := readLegacy(dir, bySlug)
	if err != nil {
		return nil, err
	}
	home, ok := bySlug[""]
	if !ok {
		return nil, errors.New("src/pages/home.html is missing")
	}
	groupOf := map[string]*Group{}
	for _, g := range groups {
		for _, p := range g.Pages {
			groupOf[p.Slug] = g
		}
	}
	for _, p := range pages {
		p.LD = jsonLD(p, home, groupOf[p.Slug])
	}

	out := map[string][]byte{}
	for _, p := range pages {
		var b bytes.Buffer
		err := tpl.ExecuteTemplate(&b, "layout.html", map[string]any{
			"Page": p, "Groups": groups, "Site": Site, "Legacy": template.JS(legacy),
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.Source, err)
		}
		name := path.Join(p.Slug, "index.html")
		if p.Slug == "" {
			name = "index.html"
		}
		out[name] = tidy(b.Bytes())
	}
	out["search.json"] = searchIndex(pages)
	out["sitemap.xml"] = sitemap(pages)
	out["robots.txt"] = []byte("User-agent: *\nAllow: /\n\nSitemap: " + Site + "/sitemap.xml\n")
	out["llms.txt"] = llms(groups, bySlug[""])
	if err := checkLinks(dir, out, bySlug); err != nil {
		return nil, err
	}
	return out, nil
}

// readNav reads src/nav.txt: "## Group icon" lines, then a page slug a line.
// Pages in src/pages that the menu does not name are built and not listed.
func readNav(dir string) ([]*Group, []*Page, error) {
	f, err := os.Open(filepath.Join(dir, "src", "nav.txt"))
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	var groups []*Group
	seen := map[string]bool{}
	var pages []*Page
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "## "):
			continue
		case strings.HasPrefix(line, "## "):
			name, icon, _ := strings.Cut(strings.TrimPrefix(line, "## "), " | ")
			groups = append(groups, &Group{Name: name, Icon: icon})
		default:
			if len(groups) == 0 {
				return nil, nil, fmt.Errorf("nav.txt: %q comes before any group", line)
			}
			if seen[line] {
				return nil, nil, fmt.Errorf("nav.txt: %q is listed twice", line)
			}
			seen[line] = true
			p, err := readPage(dir, line)
			if err != nil {
				return nil, nil, err
			}
			p.Group = groups[len(groups)-1].Name
			g := groups[len(groups)-1]
			g.Pages = append(g.Pages, p)
			pages = append(pages, p)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}
	// The rest: the home page and the pages linked to rather than listed.
	files, _ := filepath.Glob(filepath.Join(dir, "src", "pages", "*.html"))
	sort.Strings(files)
	for _, file := range files {
		slug := slugOfFile(file)
		if seen[slug] {
			continue
		}
		p, err := readPage(dir, slug)
		if err != nil {
			return nil, nil, err
		}
		p.Hidden = true
		pages = append(pages, p)
	}
	return groups, pages, nil
}

// A page's file is its slug with "--" for "/", and "home" for the root.
func slugOfFile(file string) string {
	s := strings.TrimSuffix(filepath.Base(file), ".html")
	if s == "home" {
		return ""
	}
	return strings.ReplaceAll(s, "--", "/")
}

func fileOfSlug(slug string) string {
	if slug == "" {
		return "home.html"
	}
	return strings.ReplaceAll(slug, "/", "--") + ".html"
}

// readPage reads one page: a comment of "key: value" lines, then the body.
func readPage(dir, slug string) (*Page, error) {
	file := filepath.Join(dir, "src", "pages", fileOfSlug(slug))
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	s := string(raw)
	if !strings.HasPrefix(s, "<!--") {
		return nil, fmt.Errorf("%s: begins with no header comment", file)
	}
	end := strings.Index(s, "-->")
	if end < 0 {
		return nil, fmt.Errorf("%s: the header comment never ends", file)
	}
	p := &Page{Slug: slug, Source: filepath.ToSlash(strings.TrimPrefix(file, dir+string(filepath.Separator)))}
	for _, line := range strings.Split(s[4:end], "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "title":
			p.Title = v
		case "short":
			p.Short = v
		case "seo":
			p.SEO = v
		case "description":
			p.Desc = v
		case "updated":
			p.Updated = v
		case "image":
			p.Image = v
		case "lead":
			p.Lead = v
		case "icon":
			p.Icon = v
		case "screens":
			for _, sc := range strings.Split(v, ",") {
				if sc = strings.TrimSpace(sc); sc != "" {
					p.Screens = append(p.Screens, sc)
				}
			}
		}
	}
	if p.Title == "" {
		return nil, fmt.Errorf("%s: no title", file)
	}
	if p.Short == "" {
		p.Short = p.Title
	}
	// What a search result shows: Google cuts a title at about 60 characters
	// and a snippet at about 160, so a page longer than that says less than
	// its author thinks, and one much shorter wastes the room.
	if p.SEO == "" {
		p.SEO = p.Title
	}
	if n := len([]rune(p.SEO)); n > 62 {
		return nil, fmt.Errorf("%s: the seo title is %d characters; a search result shows about 60", file, n)
	}
	if n := len([]rune(p.Desc)); n < 70 || n > 165 {
		return nil, fmt.Errorf("%s: the description is %d characters; a search snippet holds 70 to 160", file, n)
	}
	if p.Updated == "" {
		return nil, fmt.Errorf("%s: no updated date", file)
	}
	p.URL = "/" + slug
	if slug != "" {
		p.URL += "/"
	}
	p.Body = template.HTML(strings.TrimSpace(s[end+3:]))
	return p, nil
}

var (
	reHeading = regexp.MustCompile(`(?s)<h([23])((?:\s[^>]*)?)>(.*?)</h[23]>`)
	reID      = regexp.MustCompile(`\sid="([^"]+)"`)
	reTag     = regexp.MustCompile(`<[^>]+>`)
	reImg     = regexp.MustCompile(`<img\s[^>]*>`)
	reSrc     = regexp.MustCompile(`\ssrc="(/images/[^"]+)"`)
	reIcon    = regexp.MustCompile(`<icon name="([a-z0-9_-]+)"></icon>`)
)

// render fills in what the page leaves to the build: ids on headings, the
// list of sections, sizes on pictures and the icons named in the text.
func (p *Page) render(dir string, icons iconSet) error {
	body := string(p.Body)
	used := map[string]bool{}
	var err error
	body = reHeading.ReplaceAllStringFunc(body, func(m string) string {
		parts := reHeading.FindStringSubmatch(m)
		level, attrs, inner := parts[1], parts[2], parts[3]
		id := ""
		if mm := reID.FindStringSubmatch(attrs); mm != nil {
			id = mm[1]
		} else {
			id = slugify(text(inner))
			attrs += ` id="` + id + `"`
		}
		if used[id] {
			err = fmt.Errorf("two headings with the id %q", id)
		}
		used[id] = true
		if level == "2" {
			p.TOC = append(p.TOC, Heading{ID: id, Text: text(inner)})
		}
		return fmt.Sprintf(`<h%s%s>%s<a class="anchor" href="#%s" aria-hidden="true" tabindex="-1">#</a></h%s>`,
			level, attrs, inner, id, level)
	})
	if err != nil {
		return err
	}
	body = reImg.ReplaceAllStringFunc(body, func(m string) string {
		src := reSrc.FindStringSubmatch(m)
		if src == nil || strings.Contains(m, " width=") {
			return m
		}
		w, h, e := imageSize(filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(src[1], "/"))))
		if e != nil {
			err = fmt.Errorf("picture %s: %w", src[1], e)
			return m
		}
		return strings.TrimSuffix(m, ">") + fmt.Sprintf(` width="%d" height="%d" loading="lazy" decoding="async">`, w, h)
	})
	if err != nil {
		return err
	}
	body = reIcon.ReplaceAllStringFunc(body, func(m string) string {
		return string(icons.svg(reIcon.FindStringSubmatch(m)[1]))
	})
	p.Body = template.HTML(body)
	if p.Image == "" {
		if m := reSrc.FindStringSubmatch(body); m != nil {
			p.Image = m[1]
		} else {
			p.Image = "/assets/social.png"
		}
	}
	return nil
}

func text(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(reTag.ReplaceAllString(s, ""))), " ")
}

func slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// imageSize reads a PNG's or a WebP's dimensions from its header.
func imageSize(file string) (int, int, error) {
	f, err := os.Open(file)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	switch strings.ToLower(filepath.Ext(file)) {
	case ".png":
		c, err := png.DecodeConfig(f)
		return c.Width, c.Height, err
	case ".webp":
		head := make([]byte, 30)
		if _, err := f.Read(head); err != nil {
			return 0, 0, err
		}
		if string(head[0:4]) != "RIFF" || string(head[8:12]) != "WEBP" {
			return 0, 0, errors.New("not a WebP file")
		}
		switch string(head[12:16]) {
		case "VP8X":
			w := int(head[24]) | int(head[25])<<8 | int(head[26])<<16
			h := int(head[27]) | int(head[28])<<8 | int(head[29])<<16
			return w + 1, h + 1, nil
		case "VP8L":
			b := binary.LittleEndian.Uint32(head[21:25])
			return int(b&0x3fff) + 1, int(b>>14&0x3fff) + 1, nil
		case "VP8 ":
			w := int(binary.LittleEndian.Uint16(head[26:28]) & 0x3fff)
			h := int(binary.LittleEndian.Uint16(head[28:30]) & 0x3fff)
			return w, h, nil
		}
		return 0, 0, errors.New("a WebP chunk this does not read")
	}
	return 0, 0, errors.New("only PNG and WebP pictures")
}

// iconSet is the Material Symbols in assets/icons, by name.
type iconSet map[string]string

var rePath = regexp.MustCompile(`<path d="([^"]+)"`)

func readIcons(dir string) (iconSet, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.svg"))
	if err != nil {
		return nil, err
	}
	set := iconSet{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		m := rePath.FindSubmatch(raw)
		if m == nil {
			return nil, fmt.Errorf("%s has no single path", f)
		}
		name := strings.TrimSuffix(filepath.Base(f), ".svg")
		name = strings.TrimPrefix(strings.TrimPrefix(name, "sym-"), "ui-")
		set[name] = string(m[1])
	}
	return set, nil
}

func (s iconSet) svg(name string) template.HTML {
	d, ok := s[name]
	if !ok {
		panic("no icon named " + name)
	}
	return template.HTML(`<svg class="icon" viewBox="0 -960 960 960" aria-hidden="true" focusable="false"><path d="` + d + `"/></svg>`)
}

// readLegacy reads src/legacy.txt: the manual's old one-page anchors and the
// page each moved to, as a script the home page runs. A Help link from a
// Quilzo released before the manual had pages still lands where it meant.
func readLegacy(dir string, bySlug map[string]*Page) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "src", "legacy.txt"))
	if err != nil {
		return "", err
	}
	m := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || strings.HasPrefix(f[0], "#") {
			continue
		}
		slug, frag, _ := strings.Cut(f[1], "#")
		if _, ok := bySlug[slug]; !ok {
			return "", fmt.Errorf("legacy.txt: #%s moves to %q, which is not a page", f[0], slug)
		}
		to := "/" + slug + "/"
		if frag != "" {
			to += "#" + frag
		}
		m[f[0]] = to
	}
	b, _ := json.Marshal(m)
	return string(b), nil
}

// searchIndex is one entry for each section of each page.
func searchIndex(pages []*Page) []byte {
	type entry struct {
		Page    string `json:"p"`
		Section string `json:"s,omitempty"`
		URL     string `json:"u"`
		Text    string `json:"x"`
	}
	var all []entry
	reSection := regexp.MustCompile(`(?s)<h2[^>]*\sid="([^"]+)"[^>]*>(.*?)</h2>`)
	for _, p := range pages {
		if p.Slug == "" {
			continue
		}
		body := string(p.Body)
		locs := reSection.FindAllStringSubmatchIndex(body, -1)
		intro := body
		if len(locs) > 0 {
			intro = body[:locs[0][0]]
		}
		all = append(all, entry{Page: p.Title, URL: p.URL, Text: clip(p.Lead + " " + text(intro))})
		for i, l := range locs {
			end := len(body)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			all = append(all, entry{Page: p.Title, Section: strings.TrimSuffix(text(body[l[4]:l[5]]), "#"),
				URL: p.URL + "#" + body[l[2]:l[3]], Text: clip(text(body[l[1]:end]))})
		}
	}
	b, _ := json.Marshal(all)
	return append(b, '\n')
}

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 600 {
		s = s[:600]
		if i := strings.LastIndexByte(s, ' '); i > 0 {
			s = s[:i]
		}
	}
	return s
}

// jsonLD is the page described in schema.org terms: the software and the
// site on the home page, and an article with its place in the manual on
// every other. Built as data and marshalled, so it is always valid JSON.
func jsonLD(p, home *Page, g *Group) template.JS {
	abs := func(u string) string {
		if strings.HasPrefix(u, "/") {
			return Site + u
		}
		return u
	}
	project := map[string]any{
		"@type": "Organization", "@id": Site + "/#project", "name": "The Quilzo project",
		"url": "https://github.com/Quilzo", "logo": Site + "/assets/mark.png",
		"sameAs": []string{"https://github.com/Quilzo/Quilzo"},
	}
	graph := []any{project}
	if p.Slug == "" {
		graph = append(graph,
			map[string]any{
				"@type": "WebSite", "@id": Site + "/#website", "url": Site + "/", "name": "Quilzo manual",
				"description": p.Desc, "inLanguage": "en", "publisher": map[string]string{"@id": Site + "/#project"},
			},
			map[string]any{
				"@type": "SoftwareApplication", "@id": Site + "/#software", "name": "Quilzo",
				"description":            p.Desc,
				"applicationCategory":    "SecurityApplication",
				"applicationSubCategory": "AI agent control plane",
				"operatingSystem":        "Linux, macOS, Windows",
				"url":                    Site + "/",
				"downloadUrl":            "https://github.com/Quilzo/Quilzo/releases/latest",
				"license":                "https://github.com/Quilzo/Quilzo/blob/main/LICENSE",
				"isAccessibleForFree":    true,
				"offers":                 map[string]string{"@type": "Offer", "price": "0", "priceCurrency": "USD"},
				"image":                  abs(p.Image),
				"publisher":              map[string]string{"@id": Site + "/#project"},
			})
	} else {
		article := map[string]any{
			"@type": "TechArticle", "headline": p.Title, "description": p.Desc,
			"url": Site + p.URL, "mainEntityOfPage": Site + p.URL, "inLanguage": "en",
			"dateModified": p.Updated, "image": abs(p.Image),
			"author":    map[string]string{"@id": Site + "/#project"},
			"publisher": map[string]string{"@id": Site + "/#project"},
			"about":     map[string]string{"@type": "SoftwareApplication", "name": "Quilzo"},
			"isPartOf":  map[string]string{"@type": "WebSite", "name": "Quilzo manual", "url": Site + "/"},
		}
		crumbs := []any{map[string]any{"@type": "ListItem", "position": 1, "name": "Quilzo manual", "item": Site + "/"}}
		if g != nil {
			crumbs = append(crumbs, map[string]any{"@type": "ListItem", "position": 2, "name": g.Name, "item": Site + "/#" + g.ID()})
		}
		crumbs = append(crumbs, map[string]any{"@type": "ListItem", "position": len(crumbs) + 1, "name": p.Short})
		graph = append(graph, article, map[string]any{"@type": "BreadcrumbList", "itemListElement": crumbs})
	}
	b, _ := json.Marshal(map[string]any{"@context": "https://schema.org", "@graph": graph})
	return template.JS(b)
}

// llms.txt is the site described for a language model, as llmstxt.org
// proposes: what Quilzo is, and every guide with a line about it.
func llms(groups []*Group, home *Page) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Quilzo\n\n> %s\n\n", home.Desc)
	b.WriteString("Quilzo is one Go binary with no third-party dependencies, licensed AGPL-3.0-or-later or commercially. " +
		"Source: https://github.com/Quilzo/Quilzo\n")
	for _, g := range groups {
		fmt.Fprintf(&b, "\n## %s\n\n", g.Name)
		for _, p := range g.Pages {
			fmt.Fprintf(&b, "- [%s](%s%s): %s\n", p.Title, Site, p.URL, p.Desc)
		}
	}
	return b.Bytes()
}

func sitemap(pages []*Page) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, p := range pages {
		fmt.Fprintf(&b, "  <url><loc>%s%s</loc><lastmod>%s</lastmod></url>\n", Site, p.URL, p.Updated)
	}
	b.WriteString("</urlset>\n")
	return b.Bytes()
}

// tidy drops the blank lines a template leaves where its actions were.
func tidy(b []byte) []byte {
	var out bytes.Buffer
	for _, line := range bytes.Split(b, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		out.Write(bytes.TrimRight(line, " \t"))
		out.WriteByte('\n')
	}
	return out.Bytes()
}

var (
	reHref   = regexp.MustCompile(`\shref="([^"]+)"`)
	reAnyID  = regexp.MustCompile(`\sid="([^"]+)"`)
	reAnySrc = regexp.MustCompile(`\ssrc="(/[^"]+)"`)
)

// checkLinks fails the build on a link to a page or a section that is not
// there, and on a picture that is not in the repository.
func checkLinks(dir string, out map[string][]byte, bySlug map[string]*Page) error {
	ids := map[string]map[string]bool{}
	for name, body := range out {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		set := map[string]bool{}
		for _, m := range reAnyID.FindAllSubmatch(body, -1) {
			set[string(m[1])] = true
		}
		ids[name] = set
	}
	var problems []string
	for name, body := range out {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		for _, m := range reHref.FindAllSubmatch(body, -1) {
			href := html.UnescapeString(string(m[1]))
			if !strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "#") {
				continue
			}
			target, frag, _ := strings.Cut(href, "#")
			file := name
			if target != "" {
				file = strings.TrimPrefix(target, "/")
				switch {
				case out[file] != nil:
					continue
				case strings.HasPrefix(file, "demo/") || strings.HasPrefix(file, "assets/") ||
					strings.HasPrefix(file, "images/") || path.Ext(file) != "":
					if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(file))); err != nil && !strings.HasSuffix(file, "/") {
						problems = append(problems, fmt.Sprintf("%s links to %s, which is not in the repository", name, href))
					}
					continue
				case file == "" || strings.HasSuffix(file, "/"):
					file += "index.html"
				}
			}
			set, ok := ids[file]
			if !ok {
				problems = append(problems, fmt.Sprintf("%s links to %s, which is not a page", name, href))
				continue
			}
			if frag != "" && !set[frag] {
				problems = append(problems, fmt.Sprintf("%s links to %s, and that page has no #%s", name, href, frag))
			}
		}
		for _, m := range reAnySrc.FindAllSubmatch(body, -1) {
			p := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(string(m[1]), "/")))
			if _, err := os.Stat(p); err != nil {
				problems = append(problems, fmt.Sprintf("%s shows %s, which is not in the repository", name, m[1]))
			}
		}
	}
	// The pages the application sends people to.
	raw, err := os.ReadFile(filepath.Join(dir, "sections.txt"))
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		slug, frag, _ := strings.Cut(line, "#")
		if _, ok := bySlug[slug]; !ok {
			problems = append(problems, fmt.Sprintf("sections.txt names %q and there is no such page; the admin links to it", line))
			continue
		}
		if frag != "" && !ids[path.Join(slug, "index.html")][frag] {
			problems = append(problems, fmt.Sprintf("sections.txt names %q and that page has no #%s", line, frag))
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 && *draft {
		fmt.Fprintf(os.Stderr, "gen: %d broken links, carrying on (-draft)\n", len(problems))
		return nil
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d broken:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	return nil
}
