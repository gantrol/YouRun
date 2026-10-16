package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"

	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/workspace"
)

type matchedProject struct {
	engine.Project
	Score int `json:"score,omitempty"`
}

type matchedCollection struct {
	engine.Collection
	Path  string `json:"path"`
	Score int    `json:"score,omitempty"`
}

type matchedCommand struct {
	engine.Command
	ProjectName string   `json:"projectName,omitempty"`
	Collections []string `json:"collections,omitempty"`
	Score       int      `json:"score,omitempty"`
}

type searchResponse struct {
	Query       string              `json:"query"`
	Projects    []matchedProject    `json:"projects"`
	Collections []matchedCollection `json:"collections"`
	Commands    []matchedCommand    `json:"commands"`
}

func collectionPath(col engine.Collection, byID map[string]engine.Collection) string {
	names := []string{col.Name}
	seen := map[string]bool{col.ID: true}
	parent := col.Parent
	for parent != nil && *parent != "" && !seen[*parent] {
		seen[*parent] = true
		item, ok := byID[*parent]
		if !ok {
			break
		}
		names = append([]string{item.Name}, names...)
		parent = item.Parent
	}
	return strings.Join(names, " / ")
}

func searchWorkspace(service *workspace.Service, query, projectFilter string, showHidden bool) error {
	status, data, err := service.Request(context.Background(), "GET", "/api/state", nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		return errors.New("requestFailed")
	}
	var state engine.Snapshot
	if err = json.Unmarshal(data, &state); err != nil {
		return err
	}

	defs := state.Definitions
	projectByID := make(map[string]engine.Project, len(defs.Projects))
	for _, p := range defs.Projects {
		projectByID[p.ID] = p
	}
	colByID := make(map[string]engine.Collection, len(defs.Collections))
	for _, c := range defs.Collections {
		colByID[c.ID] = c
	}
	colPaths := make(map[string]string, len(defs.Collections))
	colsByCmd := make(map[string][]string)
	colIDsByCmd := make(map[string][]string)
	for _, c := range defs.Collections {
		p := collectionPath(c, colByID)
		colPaths[c.ID] = p
		for _, cmdID := range c.CommandIDs {
			colsByCmd[cmdID] = append(colsByCmd[cmdID], p)
			colIDsByCmd[cmdID] = append(colIDsByCmd[cmdID], c.ID)
		}
	}

	projFilterLower := strings.ToLower(strings.TrimSpace(projectFilter))
	matchesProjectFilter := func(p engine.Project) bool {
		if projFilterLower == "" {
			return true
		}
		return strings.EqualFold(p.ID, projFilterLower) || strings.Contains(strings.ToLower(p.Name), projFilterLower)
	}

	bestCmdScoreByProject := make(map[string]int)
	bestCmdScoreByCol := make(map[string]int)
	type scoredCmd struct {
		item  matchedCommand
		index int
	}
	var scoredCmds []scoredCmd
	for idx, cmd := range defs.Commands {
		var projName string
		var projHidden bool
		if cmd.Project != nil {
			p, ok := projectByID[*cmd.Project]
			if ok {
				if !matchesProjectFilter(p) {
					continue
				}
				projName = p.Name
				projHidden = p.Hidden
			} else if projFilterLower != "" {
				continue
			}
		} else if projFilterLower != "" {
			continue
		}
		if !showHidden && (cmd.Hidden || projHidden) {
			continue
		}
		cols := colsByCmd[cmd.ID]
		score := engine.ScoreCommand(cmd, query, projName, cols...)
		if score <= 0 {
			continue
		}
		if cmd.Project != nil && score > bestCmdScoreByProject[*cmd.Project] {
			bestCmdScoreByProject[*cmd.Project] = score
		}
		for _, colID := range colIDsByCmd[cmd.ID] {
			if score > bestCmdScoreByCol[colID] {
				bestCmdScoreByCol[colID] = score
			}
		}
		scoredCmds = append(scoredCmds, scoredCmd{
			item: matchedCommand{
				Command:     cmd,
				ProjectName: projName,
				Collections: cols,
				Score:       score,
			},
			index: idx,
		})
	}
	hasQuery := len(engine.SearchTerms(query)) > 0
	if hasQuery {
		sort.SliceStable(scoredCmds, func(i, j int) bool {
			if scoredCmds[i].item.Score != scoredCmds[j].item.Score {
				return scoredCmds[i].item.Score > scoredCmds[j].item.Score
			}
			return scoredCmds[i].index < scoredCmds[j].index
		})
	}
	commands := make([]matchedCommand, 0, len(scoredCmds))
	for _, entry := range scoredCmds {
		commands = append(commands, entry.item)
	}

	type scoredProj struct {
		item  matchedProject
		index int
	}
	var scoredProjs []scoredProj
	for idx, p := range defs.Projects {
		if (!showHidden && p.Hidden) || !matchesProjectFilter(p) {
			continue
		}
		score := engine.ScoreProject(p, query, "", bestCmdScoreByProject[p.ID])
		if score <= 0 {
			continue
		}
		scoredProjs = append(scoredProjs, scoredProj{
			item:  matchedProject{Project: p, Score: score},
			index: idx,
		})
	}
	if hasQuery {
		sort.SliceStable(scoredProjs, func(i, j int) bool {
			if scoredProjs[i].item.Score != scoredProjs[j].item.Score {
				return scoredProjs[i].item.Score > scoredProjs[j].item.Score
			}
			return scoredProjs[i].index < scoredProjs[j].index
		})
	}
	projects := make([]matchedProject, 0, len(scoredProjs))
	for _, entry := range scoredProjs {
		projects = append(projects, entry.item)
	}

	type scoredCol struct {
		item  matchedCollection
		index int
	}
	var scoredCols []scoredCol
	for idx, c := range defs.Collections {
		p := colPaths[c.ID]
		score := engine.ScoreCollection(c, p, query, bestCmdScoreByCol[c.ID])
		if score <= 0 {
			continue
		}
		scoredCols = append(scoredCols, scoredCol{
			item:  matchedCollection{Collection: c, Path: p, Score: score},
			index: idx,
		})
	}
	if hasQuery {
		sort.SliceStable(scoredCols, func(i, j int) bool {
			if scoredCols[i].item.Score != scoredCols[j].item.Score {
				return scoredCols[i].item.Score > scoredCols[j].item.Score
			}
			return scoredCols[i].index < scoredCols[j].index
		})
	}
	collections := make([]matchedCollection, 0, len(scoredCols))
	for _, entry := range scoredCols {
		collections = append(collections, entry.item)
	}

	return json.NewEncoder(os.Stdout).Encode(searchResponse{
		Query:       strings.TrimSpace(query),
		Projects:    projects,
		Collections: collections,
		Commands:    commands,
	})
}

func filterRuns(runs []engine.Run, query string) []engine.Run {
	if len(engine.SearchTerms(query)) == 0 {
		return runs
	}
	filtered := make([]engine.Run, 0, len(runs))
	for _, r := range runs {
		if engine.ScoreRun(r, query, "", nil, "") > 0 {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
