package httpapi

import (
	"bufio"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestOpenAPIMatchesRoutes keeps api/openapi.yaml in step with the route
// table: every route is documented and every documented /v1 or /mcp
// operation exists. It reads the spec's path and method keys by
// indentation, which is how the file is written.
func TestOpenAPIMatchesRoutes(t *testing.T) {
	f, err := os.Open("../../../api/openapi.yaml")
	if err != nil {
		t.Skipf("openapi.yaml not found: %v", err)
	}
	defer f.Close()

	pathLine := regexp.MustCompile(`^  (/[^:]+):\s*$`)
	methodLine := regexp.MustCompile(`^    (get|post|put|patch|delete):\s*$`)
	documented := map[string]bool{}
	inPaths, current := false, ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "paths:":
			inPaths = true
		case inPaths && len(line) > 0 && line[0] != ' ':
			inPaths = false
		case inPaths && pathLine.MatchString(line):
			current = pathLine.FindStringSubmatch(line)[1]
		case inPaths && methodLine.MatchString(line):
			documented[strings.ToUpper(methodLine.FindStringSubmatch(line)[1])+" "+current] = true
		}
	}

	registered := map[string]bool{}
	s := &Server{}
	s.routes(func(pattern string, _ access, _ handlerFunc) {
		registered[strings.Replace(pattern, "{name...}", "{name}", 1)] = true
	})

	var missing, extra []string
	for r := range registered {
		if !documented[r] {
			missing = append(missing, r)
		}
	}
	for d := range documented {
		path := strings.SplitN(d, " ", 2)[1]
		if (strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/mcp/")) && !registered[d] {
			extra = append(extra, d)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("routes missing from api/openapi.yaml:\n  %s", strings.Join(missing, "\n  "))
	}
	if len(extra) > 0 {
		t.Errorf("api/openapi.yaml documents routes that do not exist:\n  %s", strings.Join(extra, "\n  "))
	}
}
