package main

const (
	// for monitoring
	defaultMonitorIntervalSeconds = 3

	// variables
	minImageWidth  = 400
	minImageHeight = 300

	// commands
	commandStart   = "/start"
	commandCapture = "/capture"
	commandHelp    = "/help"
	commandStatus  = "/status"
	commandCancel  = "/cancel"
	commandPrivacy = "/privacy"

	// messages
	messageDefault        = "Input your command:"
	messageUnknownCommand = "Unknown command."
	messageCanceled       = "Canceled."

	// default maintenance message
	defaultMaintenanceMessage = "Service is in maintenance now."
)
