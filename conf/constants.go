package conf

const (
	// for monitoring
	DefaultMonitorIntervalSeconds = 3

	// commands
	CommandStart   = "/start"
	CommandCapture = "/capture"
	CommandHelp    = "/help"
	CommandStatus  = "/status"
	CommandCancel  = "/cancel"

	// messages
	MessageDefault        = "Input your command:"
	MessageUnknownCommand = "Unknown command."
	MessageCanceled       = "Canceled."
)
