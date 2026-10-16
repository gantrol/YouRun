package main

import (
	"encoding/base64"
	"strings"
	"time"

	"commandmanager.local/core/desktop"
	"github.com/egoist/mygo/ui"
)

// Optional metadata is loaded and decoded off the UI thread, one batch at a time.
func (a *app) loadProjectDetails(refreshID string) {
	if refreshID != "" {
		a.refreshLogoID = refreshID
	}
	if a.loadingLogos {
		a.reloadLogos = true
		return
	}
	ids := []string{}
	for _, project := range a.state.Definitions.Projects {
		if !project.Hidden {
			ids = append(ids, project.ID)
		}
	}
	refreshID = a.refreshLogoID
	a.refreshLogoID, a.reloadLogos, a.loadingLogos = "", false, true
	a.lastLogoScan = time.Now()
	previous, previousImages := a.projectDetails, a.projectLogos
	go func() {
		details := map[string]desktop.ProjectDetails{}
		images := map[string]ui.ImageSource{}
		options := map[string][]ui.ImageSource{}
		var err error
		for start := 0; start < len(ids); start += 8 {
			var batch map[string]desktop.ProjectDetails
			err = a.api.requestProject("POST", "project-details", map[string]any{"ids": ids[start:min(start+8, len(ids))], "refreshId": refreshID}, &batch)
			if err != nil {
				break
			}
			for id, detail := range batch {
				details[id] = detail
				if detail.Logo == nil {
					for _, candidate := range detail.Logos {
						if image := decodeProjectLogo(candidate.Data); image != nil {
							options[id] = append(options[id], image)
						}
						if len(options[id]) == 4 {
							break
						}
					}
				}
				if detail.Logo == nil {
					continue
				}
				if old := previous[id].Logo; old != nil && old.Data == detail.Logo.Data && previousImages[id] != nil {
					images[id] = previousImages[id]
					continue
				}
				images[id] = decodeProjectLogo(detail.Logo.Data)
			}
		}
		a.win.Update(func() {
			if a.ctx.Err() != nil {
				return
			}
			a.loadingLogos = false
			if a.reloadLogos {
				a.loadProjectDetails("")
				return
			}
			a.projectDetails, a.projectLogos = details, images
			a.projectLogoOptions = options
			if err != nil {
				a.issue = err.Error()
			}
		})
	}()
}

func decodeProjectLogo(source string) ui.ImageSource {
	kind, encoded, ok := strings.Cut(source, ",")
	if !ok {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	if kind == "data:image/svg+xml;base64" {
		if svg, err := ui.ParseSVG(data); err == nil {
			return svg
		}
	} else if bitmap, err := ui.DecodeBitmap(data); err == nil {
		return bitmap
	}
	return nil
}

func (a *app) refreshProjectDetailsOnFocus() {
	if a.loaded && !a.loadingLogos && time.Since(a.lastLogoScan) >= 30*time.Second {
		a.loadProjectDetails("")
	}
}

func (a *app) selectProjectLogo(id, logoID string) {
	detail := a.projectDetails[id]
	a.work(func() (func(), error) {
		var next struct {
			Revision int64 `json:"revision"`
		}
		err := a.api.requestProject("PUT", "project-metadata", map[string]any{"id": id, "logoId": logoID, "revision": detail.Revision}, &next)
		if err != nil && err.Error() == "conflict" {
			return func() {
				a.issue = "conflict"
				a.loadProjectDetails(id)
			}, nil
		}
		return func() {
			detail.Revision, detail.SelectedLogo = next.Revision, logoID
			detail.CustomAvatar = false
			a.projectDetails[id] = detail
			a.loadProjectDetails(id)
		}, err
	})
}

func (a *app) chooseProjectAvatar(id string) {
	detail := a.projectDetails[id]
	a.work(func() (func(), error) {
		var next struct {
			Revision  int64 `json:"revision"`
			Cancelled bool  `json:"cancelled"`
		}
		err := a.api.request("POST", "project-avatar", map[string]any{"id": id, "revision": detail.Revision}, &next)
		if err != nil && err.Error() == "conflict" {
			return func() { a.issue = "conflict"; a.loadProjectDetails(id) }, nil
		}
		if err != nil || next.Cancelled {
			return nil, err
		}
		return func() {
			detail.Revision, detail.SelectedLogo, detail.CustomAvatar = next.Revision, "", true
			if a.projectDetails == nil {
				a.projectDetails = map[string]desktop.ProjectDetails{}
			}
			a.projectDetails[id] = detail
			a.loadProjectDetails(id)
		}, nil
	})
}

// scanBusy is returned before metadata mutation, so it is safe to retry while
// the background discovery request finishes. Other failures are not replayed.
func (c *client) requestProject(method, path string, input, output any) error {
	for attempt := 0; ; attempt++ {
		if err := c.ctx.Err(); err != nil {
			return err
		}
		err := c.request(method, path, input, output)
		if err == nil || err.Error() != "scanBusy" || attempt >= 3 {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
		select {
		case <-c.ctx.Done():
			timer.Stop()
			return c.ctx.Err()
		case <-timer.C:
		}
	}
}
