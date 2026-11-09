package services

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

// legacyCategoryIconAliases are icon names the API still accepts that the web
// picker no longer offers (it maps them onto a current icon).
var legacyCategoryIconAliases = []string{"home"}

// TestCategoryIconNamesMatchWebList keeps the API's accepted icon names in
// step with CATEGORY_ICONS in apps/web/src/lib/category-icons.ts. Before this
// test the two copies were synchronised by a comment only.
func TestCategoryIconNamesMatchWebList(t *testing.T) {
	path := filepath.Join("..", "..", "..", "web", "src", "lib", "category-icons.ts")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read web category icon list: %v", err)
	}
	re := regexp.MustCompile(`\{\s*key:\s*"([a-z0-9-]+)"`)
	var web []string
	for _, m := range re.FindAllSubmatch(src, -1) {
		web = append(web, string(m[1]))
	}
	if len(web) == 0 {
		t.Fatal("no icon keys found in category-icons.ts")
	}
	slices.Sort(web)

	var api []string
	for _, n := range CategoryIconNames() {
		if !slices.Contains(legacyCategoryIconAliases, n) {
			api = append(api, n)
		}
	}
	if !slices.Equal(web, api) {
		t.Errorf("category icon lists differ\n web (%s): %v\n api: %v", path, web, api)
	}
	for _, alias := range legacyCategoryIconAliases {
		if _, ok := categoryIconNames[alias]; !ok {
			t.Errorf("legacy alias %q is no longer accepted by the API; remove it from the test", alias)
		}
	}
}
