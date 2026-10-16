package discovery

import (
	"bytes"
	"encoding/xml"
	"io"
	"path/filepath"
	"strings"
)

func (s *scan) xcode(path string, data []byte) error {
	d := xml.NewDecoder(bytes.NewReader(data))
	configuration := ""
	archive := ""
	scheme := false
	build := false
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if start, ok := tok.(xml.StartElement); ok {
			if start.Name.Local == "Scheme" {
				scheme = true
			}
			if start.Name.Local == "BuildAction" {
				build = true
			}
			for _, a := range start.Attr {
				if a.Name.Local != "buildConfiguration" {
					continue
				}
				if start.Name.Local == "LaunchAction" {
					configuration = a.Value
				}
				if start.Name.Local == "ArchiveAction" {
					archive = a.Value
				}
			}
		}
	}
	if !scheme {
		return nil
	}
	container := path
	for !strings.HasSuffix(container, ".xcodeproj") && !strings.HasSuffix(container, ".xcworkspace") {
		next := filepath.Dir(container)
		if next == container || next == "." {
			s.issue(path, "scanPartial")
			return nil
		}
		container = next
	}
	dir := filepath.Dir(container)
	flag := "-project"
	if strings.HasSuffix(container, ".xcworkspace") {
		flag = "-workspace"
	}
	args := []string{flag, "./" + filepath.Base(container), "-scheme", strings.TrimSuffix(filepath.Base(path), ".xcscheme")}
	if build {
		buildArgs := append([]string{}, args...)
		if configuration != "" {
			buildArgs = append(buildArgs, "-configuration", configuration)
		}
		s.add(path, dir, "xcodebuild", "template", "macOS", true, append(buildArgs, "build")...)
	}
	if archive != "" {
		s.add(path, dir, "xcodebuild", "template", "macOS", true, append(append([]string{}, args...), "-configuration", archive, "archive")...)
	}
	s.issue(path, "scanPartial")
	return nil
}
