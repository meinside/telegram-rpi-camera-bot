// telegram bot for using raspberry pi camera module
package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	bot "github.com/meinside/telegram-bot-go"
)

type status int16

// constants
const (
	statusWaiting status = iota

	numQueue        = 4
	numLatestPhotos = 20

	resizeKeyboard = true
)

// session struct
type _session struct {
	UserID        string
	CurrentStatus status
	LastUpdateID  int64
}

// session pool for storing individual statuses
type _sessionPool struct {
	Sessions map[string]_session
	sync.Mutex
}

// for making sure the camera is not used simultaneously
var cameraLock sync.Mutex

// capture request
type _captureRequest struct {
	UserName       string
	ChatID         interface{}
	ImageWidth     int
	ImageHeight    int
	CameraParams   map[string]interface{}
	MessageOptions map[string]interface{}
}

// variables
var apiToken string
var monitorInterval int
var isVerbose bool
var availableIds []string
var imageWidth, imageHeight int
var cameraParams map[string]interface{}
var isInMaintenance bool
var maintenanceMessage string
var pool _sessionPool
var captureChannel chan _captureRequest
var launched time.Time
var db *Database

const (
	appName = "RPiCameraBot"
)

// keyboards
var allKeyboards = [][]bot.KeyboardButton{
	bot.NewKeyboardButtons(commandCapture),
	bot.NewKeyboardButtons(commandStatus, commandHelp),
}

// loggers
var _stdout = log.New(os.Stdout, "", log.LstdFlags)
var _stderr = log.New(os.Stderr, "", log.LstdFlags)

// initialization
func init() {
	launched = time.Now()

	// read variables from config file
	if config, err := loadConfig(); err == nil {
		apiToken = config.APIToken
		availableIds = config.AvailableIds
		monitorInterval = config.MonitorInterval
		if monitorInterval <= 0 {
			monitorInterval = defaultMonitorIntervalSeconds
		}
		isVerbose = config.IsVerbose

		// image width * height
		imageWidth = config.ImageWidth
		if imageWidth < minImageWidth {
			imageWidth = minImageWidth
		}
		imageHeight = config.ImageHeight
		if imageHeight < minImageHeight {
			imageHeight = minImageHeight
		}

		// other camera params
		cameraParams = config.CameraParams

		// maintenance
		isInMaintenance = config.IsInMaintenance
		maintenanceMessage = config.MaintenanceMessage
		if len(maintenanceMessage) <= 0 {
			maintenanceMessage = defaultMaintenanceMessage
		}

		// initialize session variables
		sessions := make(map[string]_session)
		for _, v := range availableIds {
			sessions[v] = _session{
				UserID:        v,
				CurrentStatus: statusWaiting,
				LastUpdateID:  -1,
			}
		}
		pool = _sessionPool{
			Sessions: sessions,
		}

		// channels
		captureChannel = make(chan _captureRequest, numQueue)

		// local database
		db = openDB()
	} else {
		panic(err)
	}
}

// check if given Telegram id is available
func isAvailableID(id *string) bool {
	if id == nil {
		return false
	}

	for _, v := range availableIds {
		if v == *id {
			return true
		}
	}

	return false
}

// for showing help message
func getHelp() string {
	return fmt.Sprintf(`
Following commands are supported:

*For Raspberry Pi Camera Module*

%s : capture a still image with *raspistill*

*Others*

%s : show this bot's status
%s : show this help message

https://github.com/meinside/telegram-rpi-camera-bot
`,
		commandCapture,
		commandStatus,
		commandHelp,
	)
}

// for showing current status of this bot
func getStatus() string {
	return fmt.Sprintf("Uptime: %s\nMemory Usage: %s", getUptime(launched), getMemoryUsage())
}

// process incoming update from Telegram
func processUpdate(b *bot.Bot, update bot.Update, message bot.Message) bool {
	// check username
	from := update.GetFrom()
	if from != nil {
		if !isAvailableID(from.Username) {
			logError("[update] user not allowed: %+v", from.Username)
			return false
		}
	} else {
		logError("[update] user not allowed (has no `from`)")
		return false
	}

	userID := *from.Username

	// process result
	result := false

	pool.Lock()
	if session, exists := pool.Sessions[userID]; exists {
		// XXX - for skipping duplicated update
		// (sometimes same update is retrieved again and again due to Telegram's API error)
		if session.LastUpdateID != update.UpdateID {
			// save last update id
			pool.Sessions[userID] = _session{
				UserID:        session.UserID,
				CurrentStatus: session.CurrentStatus,
				LastUpdateID:  update.UpdateID,
			}

			// text from message
			var txt string
			if message.HasText() {
				txt = *message.Text
			} else {
				txt = ""
			}

			var msg string
			options := bot.OptionsSendMessage{}.
				SetReplyMarkup(replyKeyboardMarkup(resizeKeyboard)).
				SetParseMode(bot.ParseModeMarkdown)

			switch session.CurrentStatus {
			case statusWaiting:
				switch {
				// start
				case strings.HasPrefix(txt, commandStart):
					msg = messageDefault
				// capture
				case strings.HasPrefix(txt, commandCapture):
					msg = ""
				// status
				case strings.HasPrefix(txt, commandStatus):
					msg = getStatus()
				// help
				case strings.HasPrefix(txt, commandHelp):
					msg = getHelp()
				// fallback
				default:
					if len(txt) > 0 {
						msg = fmt.Sprintf("*%s*: %s", txt, messageUnknownCommand)
					} else {
						msg = messageUnknownCommand
					}
				}
			}

			if len(msg) > 0 {
				// 'typing...'
				b.SendChatAction(message.Chat.ID, bot.ChatActionTyping, nil)

				// send message
				if sent := b.SendMessage(message.Chat.ID, msg, options); sent.Ok {
					result = true
				} else {
					logError("failed to send message: %s", *sent.Description)
				}
			} else {
				if isInMaintenance {
					// send message
					if sent := b.SendMessage(message.Chat.ID, maintenanceMessage, options); sent.Ok {
						result = true
					} else {
						logError("failed to send maintenance message: %s", *sent.Description)
					}
				} else {
					// push to capture request channel
					captureChannel <- _captureRequest{
						UserName:       *message.From.Username,
						ChatID:         message.Chat.ID,
						ImageWidth:     imageWidth,
						ImageHeight:    imageHeight,
						CameraParams:   cameraParams,
						MessageOptions: options,
					}
				}
			}
		} else {
			logError("duplicated update id: %d", update.UpdateID)
		}
	} else {
		logError("session does not exist for id: %s", userID)
	}
	pool.Unlock()

	return result
}

// process capture request
func processCaptureRequest(b *bot.Bot, request _captureRequest) bool {
	// process result
	result := false

	cameraLock.Lock()
	defer cameraLock.Unlock()

	// 'typing...'
	b.SendChatAction(request.ChatID, bot.ChatActionTyping, nil)

	// send photo
	if bytes, err := captureStillImage(libCameraStillBin, request.ImageWidth, request.ImageHeight, request.CameraParams); err == nil {
		// captured time
		caption := time.Now().Format("2006-01-02 (Mon) 15:04:05")
		request.MessageOptions["caption"] = caption

		// 'uploading photo...'
		b.SendChatAction(request.ChatID, bot.ChatActionUploadPhoto, nil)

		// send photo
		if sent := b.SendPhoto(request.ChatID, bot.NewInputFileFromBytes(bytes), request.MessageOptions); sent.Ok {
			photo := sent.Result.LargestPhoto()

			db.savePhoto(request.UserName, photo.FileID, caption)

			result = true
		} else {
			msg := fmt.Sprintf("Failed to send photo: %s", *sent.Description)

			logError(msg)

			// send error message
			b.SendMessage(request.ChatID, msg, nil)
		}
	} else {
		message := fmt.Sprintf("Image capture failed: %s", err)

		logError(message)

		b.SendMessage(request.ChatID, message, request.MessageOptions)
	}

	return result
}

// process inline query
func processInlineQuery(b *bot.Bot, update bot.Update, inlineQuery bot.InlineQuery) bool {
	// check username
	from := update.GetFrom()
	if from != nil {
		if !isAvailableID(from.Username) {
			logError("[inline query] user not allowed: %+v", from.Username)
			return false
		}
	} else {
		logError("[inline query] user not allowed (has no `from`)")
		return false
	}

	userID := *from.Username

	// retrieve cached photos,
	photos := db.getPhotos(userID, numLatestPhotos)

	if len(photos) > 0 {
		photoResults := []interface{}{}

		// build up inline query results with cached photos,
		for _, photo := range photos {
			caption := photo.Caption

			if newPhoto, id := bot.NewInlineQueryResultCachedPhoto(photo.FileId); id != nil {
				newPhoto.Caption = &caption

				photoResults = append(photoResults, newPhoto)
			}
		}

		// then answer inline query
		sent := b.AnswerInlineQuery(
			inlineQuery.ID,
			photoResults,
			nil,
		)

		if sent.Ok {
			return true
		}

		logError("failed to answer inline query: %s", *sent.Description)
	} else {
		logError("no cached photos for inline query.")
	}

	return false
}

// keyboard markup for reply
func replyKeyboardMarkup(resize bool) bot.ReplyKeyboardMarkup {
	return bot.NewReplyKeyboardMarkup(allKeyboards).
		SetResizeKeyboard(resize)
}

func main() {
	client := bot.NewClient(apiToken)
	client.Verbose = isVerbose

	// get info about this bot
	if me := client.GetMe(); me.Ok {
		logMessage("starting bot: @%s (%s)", *me.Result.Username, me.Result.FirstName)

		// delete webhook (getting updates will not work when wehbook is set up)
		if unhooked := client.DeleteWebhook(false); unhooked.Ok {
			// monitor request capture channel
			go func() {
				for request := range captureChannel {
					// do capture and send response
					processCaptureRequest(client, request)
				}
			}()

			// handle updates
			client.SetMessageHandler(func(b *bot.Bot, update bot.Update, message bot.Message, edited bool) {
				processUpdate(b, update, message)
			})
			client.SetInlineQueryHandler(func(b *bot.Bot, update bot.Update, inlineQuery bot.InlineQuery) {
				processInlineQuery(b, update, inlineQuery)
			})

			// start polling
			client.StartPollingUpdates(0, monitorInterval, func(b *bot.Bot, update bot.Update, err error) {
				// NOTE: actual updates are handled through handlers above

				if err != nil {
					logError("error while receiving update (%s)", err)
				}
			})
		} else {
			panic("failed to delete webhook")
		}
	} else {
		panic("failed to get info of the bot")
	}
}

func logMessage(format string, a ...interface{}) {
	_stdout.Printf(format, a...)
}

func logError(format string, a ...interface{}) {
	_stderr.Printf(format, a...)
}
