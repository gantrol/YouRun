package discovery

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
)

type nodePackage struct {
	Name           string            `json:"name"`
	Scripts        map[string]string `json:"scripts"`
	PackageManager string            `json:"packageManager"`
}

func (s *scan) node(path string, data []byte, rule Rule) error {
	var pkg nodePackage
	if err := json.Unmarshal(data, &pkg); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if dir == "." && pkg.Name != "" && len(pkg.Name) <= 200 {
		s.result.Name = pkg.Name
	}
	manager, managerReview := s.nodePackageManager(path, pkg, rule)
	if manager == "" {
		return nil
	}
	names := make([]string, 0, len(pkg.Scripts))
	for name, value := range pkg.Scripts {
		if strings.TrimSpace(value) != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if strings.HasPrefix(name, "-") {
			s.issue(path, "scanInvalidManifest")
			continue
		}
		before := len(s.result.Candidates)
		s.addRuleCommand(rule, "script", path, dir, map[string]string{"packageManager": manager, "scriptName": name}, "", managerReview)
		if len(s.result.Candidates) > before && len(name) <= 200 {
			s.result.Candidates[len(s.result.Candidates)-1].Title = name
		}
	}
	return nil
}

// Keep package scripts and supplemental framework commands on the same manager.
func (s *scan) nodePackageManager(path string, pkg nodePackage, rule Rule) (string, bool) {
	dir := filepath.Dir(path)
	manager := ""
	managerReview := false
	for current := dir; ; current = filepath.Dir(current) {
		declared := pkg.PackageManager
		if current != dir {
			var parent nodePackage
			if raw := s.read(filepath.Join(current, "package.json")); raw != nil {
				_ = json.Unmarshal(raw, &parent)
			}
			declared = parent.PackageManager
		}
		if declared != "" {
			candidate, _, _ := strings.Cut(declared, "@")
			if containsString(rule.Options.PackageManagers, candidate) {
				manager = candidate
			}
			break
		}
		found := map[string]bool{}
		for file, tool := range rule.Options.Lockfiles {
			if s.entries[filepath.Join(current, file)] {
				found[tool] = true
			}
		}
		if len(found) > 1 {
			s.issue(path, "scanManagerUnknown")
			return "", false
		}
		for tool := range found {
			manager = tool
		}
		if manager != "" || current == "." {
			break
		}
	}
	if manager == "" {
		manager = rule.Options.DefaultManager
		managerReview = true
		s.issue(path, "scanManagerUnknown")
	}
	return manager, managerReview
}

func (s *scan) golang(path string, data []byte) error {
	dir := filepath.Dir(path)
	if filepath.Base(path) == "go.work" {
		work, err := modfile.ParseWork(path, data, nil)
		if err != nil {
			return err
		}
		for _, use := range work.Use {
			member := filepath.Clean(filepath.Join(dir, filepath.FromSlash(use.Path)))
			if !filepath.IsLocal(member) || !s.entries[filepath.Join(member, "go.mod")] {
				s.issue(path, "scanPartial")
			}
		}
		return nil
	}
	module, err := modfile.ParseLax(path, data, nil)
	if err != nil {
		return err
	}
	if module.Module == nil {
		s.issue(path, "scanInvalidManifest")
		return nil
	}
	for _, args := range [][]string{{"build", "./..."}, {"test", "./..."}, {"vet", "./..."}} {
		s.add(path, dir, "go", "template", "", false, args...)
	}
	// Read syntax only. Never call go list, load imports or run go:generate.
	mains := map[string]bool{}
	files := make([]string, 0)
	for entry := range s.entries {
		if strings.HasSuffix(entry, ".go") && !strings.HasSuffix(entry, "_test.go") && s.nearest(entry, "go.mod") == path {
			files = append(files, entry)
		}
	}
	sort.Strings(files)
	for _, file := range files {
		fileDir := filepath.Dir(file)
		if mains[fileDir] {
			continue
		}
		raw := s.read(file)
		if raw == nil {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, raw, parser.SkipObjectResolution)
		if err != nil {
			s.issue(file, "scanPartial")
			continue
		}
		if parsed.Name.Name != "main" {
			continue
		}
		for _, decl := range parsed.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "main" && fn.Type.Params.NumFields() == 0 && fn.Type.Results.NumFields() == 0 && fn.Type.TypeParams.NumFields() == 0 {
				mains[fileDir] = true
				rel, _ := filepath.Rel(dir, fileDir)
				target := "."
				if rel != "." {
					target = "./" + filepath.ToSlash(rel)
				}
				review := strings.Contains(string(raw), "//go:build") || strings.Contains(string(raw), "// +build") || strings.Contains(strings.TrimSuffix(filepath.Base(file), ".go"), "_")
				s.add(path, dir, "go", "template", "", review, "run", target)
				break
			}
		}
	}
	return nil
}
func (s *scan) nearest(path, name string) string {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		p := filepath.Join(dir, name)
		if s.entries[p] {
			return p
		}
		if dir == "." {
			return ""
		}
	}
}
