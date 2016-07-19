package conf

const (
	// for monitoring
	DefaultMonitorIntervalSeconds = 3

	// variables
	MinImageWidth  = 400
	MinImageHeight = 300

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

	// default maintenance message
	DefaultMaintenanceMessage = "Service is in maintenance now."
)
