package workspace

import (
	"commandmanager.local/core/internal/database"
	"commandmanager.local/core/internal/discovery"
	"commandmanager.local/core/internal/engine"
	"context"
	"encoding/json"
	"errors"
)

func (s *Service) importProject(ctx context.Context, body []byte) (int, []byte, error) {
	var request struct {
		Directory    string    `json:"directory"`
		Revision     int64     `json:"revision"`
		CandidateIDs *[]string `json:"candidateIds,omitempty"`
	}
	if !decode(body, &request) {
		return failure(400, "invalidRequest")
	}
	result, err := discovery.Discover(ctx, request.Directory)
	if err != nil {
		code := "scanFailed"
		if err.Error() == "invalidDirectory" {
			code = "invalidDirectory"
		}
		return failure(400, code)
	}
	selected := map[string]bool{}
	if request.CandidateIDs != nil {
		for _, id := range *request.CandidateIDs {
			selected[id] = true
		}
	}
	candidates := []discovery.Candidate{}
	for _, c := range result.Candidates {
		if (request.CandidateIDs == nil && !c.Review) || selected[c.ID] {
			candidates = append(candidates, c)
		}
		delete(selected, c.ID)
	}
	if len(selected) > 0 {
		return failure(400, "invalidRequest")
	}
	if len(candidates) == 0 {
		return failure(400, "noProjectCommands")
	}
	state, err := s.snapshot(ctx)
	if err != nil {
		return failure(500, "storageFailed")
	}
	if state.Definitions.Revision != request.Revision {
		return failure(409, "conflict")
	}
	commands := append([]engine.Command{}, state.Definitions.Commands...)
	projectID := ""
	newCandidates := []discovery.Candidate{}
	for _, c := range candidates {
		exists := false
		for _, old := range commands {
			if old.ID == c.ID || (old.Source != nil && *old.Source == c.Source && old.Text == c.Text) {
				exists = true
				if old.Project != nil && projectID == "" {
					projectID = *old.Project
				}
				break
			}
		}
		if !exists {
			newCandidates = append(newCandidates, c)
		}
	}
	if len(newCandidates) == 0 {
		return failure(409, "alreadyImported")
	}
	next := state.Definitions
	if projectID == "" {
		projectID = engine.ID()
		p := engine.Project{ID: projectID, Name: result.Name, Buttons: []engine.Button{{ID: engine.ID(), CommandID: "", Icon: "play"}}}
		next.Projects = append(append([]engine.Project{}, next.Projects...), p)
	}
	for _, c := range newCandidates {
		source := c.Source
		commands = append(commands, engine.Command{ID: c.ID, Title: c.Title, Text: c.Text, Project: &projectID, Source: &source, Shell: c.Shell, Platform: c.Platform, Directory: c.Directory, Fields: []string{}, Icon: c.Icon})
	}
	next.Commands = commands
	if err = engine.ValidateDefinitions(next); err != nil {
		return failure(400, err.Error())
	}
	data, err := json.Marshal(next)
	if err != nil {
		return 0, nil, err
	}
	saved, err := s.store.SaveDefinitions(ctx, next.Revision, data)
	if errors.Is(err, database.ErrConflict) {
		return failure(409, "conflict")
	}
	if err != nil {
		return failure(500, "storageFailed")
	}
	return 200, saved, nil
}
