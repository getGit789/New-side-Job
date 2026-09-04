package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// Automated accessibility check (plan §6.4): every control has a programmatic name,
// the document declares its language, and there is a landmark for the main content.
// Keyboard use follows from using only native links, forms and buttons (no scripts).
func TestTemplatesAreAccessible(t *testing.T) {
	control := regexp.MustCompile(`<(input|select|textarea)\b[^>]*>`)
	labelOpen := regexp.MustCompile(`<label\b[^>]*>$`)
	attr := func(tag, name string) string {
		m := regexp.MustCompile(name + `="([^"]*)"`).FindStringSubmatch(tag)
		if m == nil {
			return ""
		}
		return m[1]
	}
	fs.WalkDir(templateFS, "templates", func(path string, e fs.DirEntry, _ error) error {
		if e.IsDir() {
			return nil
		}
		src, _ := templateFS.ReadFile(path)
		html := string(src)
		if path == "templates/layout.html" {
			if !strings.Contains(html, `<html lang="`) || !strings.Contains(html, "<main") {
				t.Errorf("%s: needs <html lang> and a <main> landmark", path)
			}
		}
		for _, loc := range control.FindAllStringIndex(html, -1) {
			tag := html[loc[0]:loc[1]]
			if attr(tag, "type") == "hidden" || attr(tag, "aria-label") != "" {
				continue
			}
			wrapped := labelOpen.MatchString(strings.TrimSpace(html[:loc[0]]))
			id := attr(tag, "id")
			if !wrapped && (id == "" || !strings.Contains(html, `for="`+id+`"`)) {
				t.Errorf("%s: control has no label: %s", path, tag)
			}
		}
		return nil
	})
}
