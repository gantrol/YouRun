package database

import (
	"fmt"
	"strings"
)

// The column names are also the stable JSON contract. Arrays remain JSON values;
// editable scalar fields are ordinary SQLite columns.
type column struct {
	name, declaration, fallback string
	json, boolean               bool
}

type table struct {
	name, scope string
	columns     []column
	ordered     bool
}

func text(name string) column {
	return column{name: name, declaration: "TEXT NOT NULL DEFAULT ''", fallback: "''"}
}
func nullable(name string) column { return column{name: name, declaration: "TEXT", fallback: "NULL"} }
func number(name string) column {
	return column{name: name, declaration: "INTEGER NOT NULL DEFAULT 0", fallback: "0"}
}
func boolean(name string) column {
	c := number(name)
	c.boolean = true
	c.declaration += " CHECK(" + name + " IN (0,1))"
	return c
}
func array(name string) column {
	return column{name: name, declaration: "TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(\"" + name + "\") AND json_type(\"" + name + "\")='array')", fallback: "'[]'", json: true}
}
func object(name string) column {
	return column{name: name, declaration: "TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(\"" + name + "\") AND json_type(\"" + name + "\")='object')", fallback: "'{}'", json: true}
}
func nullableObject(name string) column {
	return column{name: name, declaration: "TEXT DEFAULT NULL CHECK(\"" + name + "\" IS NULL OR (json_valid(\"" + name + "\") AND json_type(\"" + name + "\")='object'))", fallback: "NULL", json: true}
}

var projects = table{"projects", "definitions", []column{text("name"), array("buttons"), boolean("hidden")}, true}
var commandsV3 = table{"commands", "definitions", []column{
	text("title"), text("text"), nullable("project"), nullable("source"), text("shell"), text("platform"), text("directory"), array("fields"), boolean("favorite"), text("icon"), boolean("hidden"), boolean("persistent"), array("aliases"),
}, true}
var collections = table{"collections", "definitions", []column{text("name"), nullable("parent"), array("commandIds")}, true}
var runsV3 = table{"runs", "runs", []column{
	text("commandId"), text("title"), text("text"), text("template"), text("directory"), object("values"), text("shell"), text("time"), text("status"), number("pid"), {name: "exitCode", declaration: "INTEGER", fallback: "NULL"},
}, true}
var metadataV1 = table{"project_metadata", "metadata", []column{array("roots"), text("logoId"), text("avatar"), number("revision")}, false}
var metadata = table{"project_metadata", "metadata", []column{array("roots"), array("locations"), text("activeLocationId"), text("logoId"), text("avatar"), number("revision")}, false}

// Keep the previously executed migration's column lists unchanged.
var commands = table{"commands", "definitions", append(commandsV3.columns, boolean("omitOutput")), true}
var runsV4 = table{"runs", "runs", append(runsV3.columns, boolean("omitOutput")), true}
var runs = table{"runs", "runs", append(runsV4.columns, nullableObject("launchSource")), true}

func (t table) schema() string {
	parts := []string{"id TEXT PRIMARY KEY NOT NULL CHECK(length(id) BETWEEN 1 AND 80)"}
	for _, c := range t.columns {
		parts = append(parts, `"`+c.name+`" `+c.declaration)
	}
	if t.ordered {
		parts = append(parts, "position INTEGER NOT NULL DEFAULT 0")
	}
	sql := "CREATE TABLE " + t.name + " (" + strings.Join(parts, ",") + ");"
	for _, operation := range []string{"INSERT", "UPDATE", "DELETE"} {
		sql += fmt.Sprintf("CREATE TRIGGER %s_%s AFTER %s ON %s BEGIN UPDATE revisions SET version=version+1 WHERE scope='%s'; END;", t.name, strings.ToLower(operation), operation, t.name, t.scope)
	}
	return sql
}

func (t table) rowJSON() string {
	parts := []string{"'id',id"}
	for _, c := range t.columns {
		value := `"` + c.name + `"`
		if c.json {
			value = "json(" + value + ")"
		}
		if c.boolean {
			value = "json(CASE WHEN " + value + " THEN 'true' ELSE 'false' END)"
		}
		parts = append(parts, "'"+c.name+"',"+value)
	}
	return "json_object(" + strings.Join(parts, ",") + ")"
}

func (t table) listSQL() string {
	order := "id"
	if t.ordered {
		order = "position,id"
	}
	if t.name == "runs" {
		order = `"time" DESC,rowid DESC`
	}
	return "SELECT coalesce(json_group_array(json(record)),'[]') FROM (SELECT " + t.rowJSON() + " AS record FROM " + t.name + " ORDER BY " + order + ")"
}

// One set-oriented statement per table, even for a large imported workspace.
func (t table) upsertSQL() string {
	names, values, updates, differences := []string{"id"}, []string{"json_extract(value,'$.id')"}, []string{}, []string{}
	for _, c := range t.columns {
		name := `"` + c.name + `"`
		names = append(names, name)
		values = append(values, "coalesce(json_extract(value,'$."+c.name+"'),"+c.fallback+")")
		updates = append(updates, name+"=excluded."+name)
		differences = append(differences, t.name+"."+name+" IS NOT excluded."+name)
	}
	if t.ordered {
		names = append(names, "position")
		values = append(values, "CAST(key AS INTEGER)")
		updates = append(updates, "position=excluded.position")
		differences = append(differences, t.name+".position IS NOT excluded.position")
	}
	return "INSERT INTO " + t.name + " (" + strings.Join(names, ",") + ") SELECT " + strings.Join(values, ",") + " FROM json_each(?) WHERE type='object' ON CONFLICT(id) DO UPDATE SET " + strings.Join(updates, ",") + " WHERE " + strings.Join(differences, " OR ")
}
