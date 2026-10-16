package desktop

import (
	"commandmanager.local/core/resources"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

type ClosePrompt struct {
	Title         string
	Message       string
	Detail        string
	Keep          string
	Stop          string
	Cancel        string
	StopError     string
	StopErrorBody string
	Acknowledge   string
	StandardKeys  string
	CanStop       bool
}

func ClosePromptFor(activity Activity, unavailable bool) ClosePrompt {
	locale := activity.Locale
	if locale != "en" {
		locale = "zh"
	}
	m := resources.Messages(locale)
	printer := message.NewPrinter(language.Make(locale))
	prompt := ClosePrompt{Keep: m["quitKeep"], Cancel: m["cancel"], Acknowledge: m["quitAcknowledge"]}
	if unavailable {
		prompt.Title, prompt.Message, prompt.Detail = m["quitUnavailableTitle"], m["quitUnavailableMessage"], m["quitUnavailableDetail"]
		prompt.StandardKeys = m["quitUnavailableKeys"]
		return prompt
	}
	prompt.Title = m["quitTitle"]
	prompt.Message = printer.Sprintf(m["quitManagedMessage"], m["productName"], activity.Active)
	prompt.Detail, prompt.Stop = m["quitManagedDetail"], m["quitStop"]
	if activity.Unknown > 0 {
		prompt.Message = printer.Sprintf(m["quitUnknownMessage"], activity.Active, activity.Unknown)
		prompt.Detail, prompt.Stop = m["quitUnknownDetail"], m["quitStopManaged"]
	}
	prompt.StopError, prompt.StopErrorBody = m["quitStopError"], m["quitStopErrorBody"]
	prompt.StandardKeys = printer.Sprintf(m["quitStandardKeys"], prompt.Stop)
	prompt.CanStop = activity.Active > 0
	return prompt
}
