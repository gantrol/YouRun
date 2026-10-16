package main

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"commandmanager.local/core/desktop"
	"commandmanager.local/core/internal/discovery"
	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/resources"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type page struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}
type session struct {
	Query       string                       `json:"query"`
	Expanded    map[string]bool              `json:"expanded"`
	Pages       []page                       `json:"pages"`
	Directories map[string]string            `json:"directories"`
	Inputs      map[string]map[string]string `json:"inputs"`
}
type updateConfig struct {
	Repository string `json:"repository"`
	PublicKey  string `json:"publicKey"`
}
type updateState struct {
	Disabled      bool         `json:"disabled"`
	StoreManaged  bool         `json:"storeManaged"`
	Phase         string       `json:"phase"`
	Version       string       `json:"version"`
	TargetVersion string       `json:"targetVersion"`
	Error         string       `json:"error"`
	Config        updateConfig `json:"config"`
	LocalSource   string       `json:"localSource"`
	LocalRestart  bool         `json:"localRestart"`
	BuildLog      string       `json:"buildLog"`
}
type app struct {
	win        *mygo.Window
	host       *desktop.Host
	api        *client
	repository *workspaceRepository
	ctx        context.Context
	cancel     context.CancelFunc
	workspaceViewModel
	runOutputViewModel
	session
	catalogs                                                    map[string]map[string]string
	loaded, busy, ready, candidate, leaving, closeAfterDiscard  bool
	bootstrapping                                               bool
	issue                                                       string
	commandIssues                                               map[string]string
	workspaceCancel, logCancel, appearanceCancel, updatesCancel context.CancelFunc
	logRun                                                      string
	appearanceRevision, appearanceRequested                     int64
	rebuilding                                                  atomic.Bool
	approved, prompting                                         atomic.Bool
	pendingPage                                                 *page
	updates                                                     updateState
	updateDraft                                                 updateConfig
	updateEditing                                               bool
	updateLoaded                                                bool
	scan                                                        discovery.Result
	importScanned                                               bool
	checked                                                     map[string]bool
	importProject                                               string
	searchFocus                                                 bool
	searchTerm                                                  string
	searchExpanded                                              map[string]bool
	theme                                                       *ui.Theme
	catalog                                                     *componentCatalogState
	skin                                                        string
	projectDetails                                              map[string]desktop.ProjectDetails
	projectOpeners                                              []desktop.ProjectOpener
	loadingOpeners                                              bool
	openersError, editorExecutable                              string
	projectLogos                                                map[string]ui.ImageSource
	projectLogoOptions                                          map[string][]ui.ImageSource
	lastLogoScan                                                time.Time
	importMetadataProject, importMetadataDirectory              string
	loadingLogos, reloadLogos                                   bool
	refreshLogoID                                               string
	quitOpen, quitBusy, quitFocus, quitError                    bool
	quitPrompt                                                  desktop.ClosePrompt
	startup                                                     desktop.StartupSettings
	startupLoaded, loadingStartup, savingStartup                bool
	startupError                                                string
}

func newApp(host *desktop.Host) *app {
	ctx, cancel := context.WithCancel(context.Background())
	a := &app{host: host, ctx: ctx, cancel: cancel, api: newClient(ctx, host.URL),
		catalogs: map[string]map[string]string{"en": resources.Messages("en"), "zh": resources.Messages("zh")}, skin: "comic", theme: appTheme("comic"),
		session: session{Expanded: map[string]bool{}, Directories: map[string]string{}, Inputs: map[string]map[string]string{}}}
	a.state.Definitions.Locale = "zh"
	a.repository = &workspaceRepository{api: a.api}
	a.appearanceRevision, a.appearanceRequested = -1, -1
	return a
}
func (a *app) m(key string) string {
	catalog := a.catalogs[a.state.Definitions.Locale]
	if catalog == nil {
		catalog = a.catalogs["en"]
	}
	if text := catalog[key]; text != "" {
		return text
	}
	return key
}
func (a *app) current() page {
	if len(a.Pages) == 0 {
		return page{Kind: "home"}
	}
	return a.Pages[len(a.Pages)-1]
}
func (a *app) open(p page) {
	if a.busy {
		return
	}
	if a.dirty() {
		a.pendingPage = &p
		a.closeAfterDiscard = false
		a.leaving = true
		return
	}
	current := a.current()
	if (current.Kind == "command" || current.Kind == "run") && (p.Kind == "command" || p.Kind == "run") {
		a.Pages[len(a.Pages)-1] = p
		a.changedPage()
		return
	}
	a.Pages = append(a.Pages, p)
	a.leaving = false
	a.issue = ""
	a.changedPage()
}
func (a *app) changedPage() {
	p := a.current()
	if p.Kind == "settings" {
		a.loadStartup()
	}
	id := ""
	if p.Kind == "run" {
		id = p.ID
	}
	a.subscribeLog(id)
	if p.Kind == "updates" || p.Kind == "settings" {
		a.observeUpdates()
	} else if !a.rebuilding.Load() && a.updatesCancel != nil {
		a.updatesCancel()
	}
}
func (a *app) back(discard bool) {
	if a.busy {
		return
	}
	a.pendingPage = nil
	if a.dirty() && !discard {
		a.leaving = true
		return
	}
	a.discardDraft()
	if len(a.Pages) > 0 {
		a.Pages = a.Pages[:len(a.Pages)-1]
	}
	a.changedPage()
}
func (a *app) discardDraft() {
	a.commandDraft = nil
	a.projectDraft = nil
	a.draftOriginal = ""
	a.draftConflict = false
	a.updateEditing = false
	a.leaving = false
}
func encoded(value any) string { data, _ := json.Marshal(value); return string(data) }
func (a *app) dirty() bool {
	if a.commandDraft != nil {
		return encoded(a.commandDraft) != a.draftOriginal
	}
	if a.projectDraft != nil {
		return encoded(a.projectDraft) != a.draftOriginal
	}
	return a.updateEditing && a.updateDraft != a.updates.Config
}
func cloneDefinitions(d engine.Definitions) engine.Definitions {
	d.Projects = slices.Clone(d.Projects)
	d.Commands = slices.Clone(d.Commands)
	d.Collections = slices.Clone(d.Collections)
	for i := range d.Collections {
		d.Collections[i].CommandIDs = slices.Clone(d.Collections[i].CommandIDs)
	}
	for i := range d.Projects {
		d.Projects[i].Buttons = slices.Clone(d.Projects[i].Buttons)
	}
	for i := range d.Commands {
		d.Commands[i].Fields = slices.Clone(d.Commands[i].Fields)
		d.Commands[i].Aliases = slices.Clone(d.Commands[i].Aliases)
	}
	return d
}

// All model mutations happen on MyGo's UI thread; disk/network work happens in
// bounded background operations. One mutation is admitted at a time.
func (a *app) work(task func() (func(), error)) {
	if a.busy || a.candidate || a.host.UpdatePending() {
		return
	}
	a.busy = true
	a.issue = ""
	go func() {
		finish, err := task()
		a.win.Update(func() {
			if a.ctx.Err() != nil {
				return
			}
			a.busy = false
			defer a.flushWorkspace()
			if err != nil {
				a.issue = err.Error()
				return
			}
			if finish != nil {
				finish()
			}
		})
	}()
}
func (a *app) save(d engine.Definitions, finish func()) {
	engine.PruneCollectionCommands(&d)
	if a.draftConflict {
		a.issue = "conflict"
		return
	}
	a.work(func() (func(), error) {
		var next engine.Definitions
		if err := a.api.request("PUT", "definitions", d, &next); err != nil {
			return nil, err
		}
		return func() {
			if next.Revision >= a.state.Definitions.Revision {
				a.state.Definitions = next
			}
			a.loadProjectDetails("")
			if finish != nil {
				finish()
			}
		}, nil
	})
}
func (a *app) bootstrap() {
	if a.bootstrapping {
		return
	}
	a.bootstrapping = true
	if a.workspaceCancel != nil {
		a.workspaceCancel()
	}
	if a.appearanceCancel != nil {
		a.appearanceCancel()
	}
	a.appearanceRevision, a.appearanceRequested = -1, -1
	a.pending = nil
	go func() {
		appearance := desktop.Appearance{Skin: "comic"}
		appearanceResult := make(chan error, 1)
		go func() { appearanceResult <- a.api.request("GET", "appearance", nil, &appearance) }()
		state, err := a.repository.read(a.ctx)
		var result struct {
			Resume    *session `json:"resume"`
			Candidate bool     `json:"candidate"`
		}
		if err == nil {
			err = a.api.request("GET", "updates/session", nil, &result)
		}
		appearanceErr := <-appearanceResult
		a.win.Update(func() {
			if a.ctx.Err() != nil {
				return
			}
			a.bootstrapping = false
			if err != nil {
				a.issue = err.Error()
				if a.loaded {
					a.subscribeWorkspace()
				}
				return
			}
			a.workspaceViewModel.accept(state)
			a.skin = appearance.Skin
			a.theme = appTheme(a.skin)
			if appearanceErr != nil {
				a.issue = "skinLoadFailed"
			}
			a.loaded = true
			a.ready = false
			a.candidate = result.Candidate
			if result.Resume != nil {
				a.session = *result.Resume
			}
			a.refreshDrafts()
			if a.Expanded == nil {
				a.Expanded = map[string]bool{}
			}
			if a.Inputs == nil {
				a.Inputs = map[string]map[string]string{}
			}
			if a.Directories == nil {
				a.Directories = map[string]string{}
			}
			a.changedPage()
			a.loadProjectDetails("")
			a.loadProjectOpeners(false)
			a.subscribeWorkspace()
			a.observeUpdates()
		})
	}()
}
func active(run engine.Run) bool { return run.Status == "running" || run.Status == "stopping" }
func (a *app) running(id string) *engine.Run {
	for _, r := range a.state.Runs {
		if r.CommandID == id && active(r) {
			return &r
		}
	}
	return nil
}
func (a *app) directory(c engine.Command) string {
	if d, ok := a.Directories[c.ID]; ok {
		return d
	}
	return c.Directory
}
func (a *app) launch(c engine.Command, details bool) {
	if run := a.running(c.ID); run != nil {
		a.open(page{Kind: "run", ID: run.ID})
		return
	}
	if !details && (len(c.Fields) > 0 || strings.TrimSpace(a.directory(c)) == "") {
		a.openCommand(c.ID)
		return
	}
	values := a.Inputs[c.ID]
	if strings.TrimSpace(a.directory(c)) == "" {
		a.setCommandIssue(c.ID, "fieldError")
		return
	}
	for _, field := range c.Fields {
		if strings.TrimSpace(values[field]) == "" {
			a.setCommandIssue(c.ID, "required")
			return
		}
	}
	request := engine.Launch{ID: engine.ID(), CommandID: c.ID, Directory: a.directory(c), Values: values}
	delete(a.commandIssues, c.ID)
	a.work(func() (func(), error) {
		err := a.api.request("POST", "run", request, nil)
		if err != nil && err.Error() == "alreadyRunning" {
			// Reconcile a start accepted by another window or a lost response.
			var state engine.Snapshot
			if refreshErr := a.api.request("GET", "state", nil, &state); refreshErr == nil {
				return func() { a.receiveWorkspace(state) }, nil
			} else {
				err = refreshErr
			}
		}
		return func() {
			if err != nil {
				a.setCommandIssue(c.ID, err.Error())
				return
			}
		}, nil
	})
}
func (a *app) stop(id string) {
	a.work(func() (func(), error) {
		err := a.api.request("POST", "stop", map[string]string{"id": id}, nil)
		return nil, err
	})
}
func (a *app) replay(id string) {
	if a.dirty() {
		a.issue = "unsaved"
		return
	}
	a.work(func() (func(), error) {
		var result struct {
			engine.Run
			SelfRestart bool `json:"selfRestart"`
		}
		err := a.api.request("POST", "replay", map[string]string{"id": engine.ID(), "runId": id}, &result)
		return func() {
			if result.SelfRestart {
				if a.dirty() {
					a.issue = "updateUnsaved"
					return
				}
				a.open(page{Kind: "updates"})
				a.updateAction("rebuild", nil)
				return
			}
			run := result.Run
			if a.current() == (page{Kind: "run", ID: id}) {
				a.Pages[len(a.Pages)-1].ID = run.ID
				a.changedPage()
			}
		}, err
	})
}
func (a *app) openCommand(id string) {
	if a.Inputs[id] == nil {
		a.Inputs[id] = map[string]string{}
	}
	a.open(page{Kind: "command", ID: id})
}
