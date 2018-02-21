// telegram bot for using raspberry pi camera module
package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	bot "github.com/meinside/telegram-bot-go"

	"github.com/meinside/telegram-bot-rpi-camera/conf"
	"github.com/meinside/telegram-bot-rpi-camera/helper"

	"github.com/meinside/loggly-go"
)

type Status int16

const (
	StatusWaiting Status = iota
)

const (
	NumQueue        = 4
	NumLatestPhotos = 20
)

type Session struct {
	UserId        string
	CurrentStatus Status
	LastUpdateId  int
}

// session pool for storing individual statuses
type SessionPool struct {
	Sessions map[string]Session
	sync.Mutex
}

// for making sure the camera is not used simultaneously
var cameraLock sync.Mutex

type CaptureRequest struct {
	UserName       string
	ChatId         interface{}
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
var pool SessionPool
var captureChannel chan CaptureRequest
var launched time.Time
var logger *loggly.Loggly
var db *helper.Database

const (
	AppName = "RPiCameraBot"
)

type LogglyLog struct {
	Application string      `json:"app"`
	Severity    string      `json:"severity"`
	Message     string      `json:"message,omitempty"`
	Object      interface{} `json:"obj,omitempty"`
}

// keyboards
var allKeyboards = [][]bot.KeyboardButton{
	bot.NewKeyboardButtons(conf.CommandCapture),
	bot.NewKeyboardButtons(conf.CommandStatus, conf.CommandHelp),
}
var cancelKeyboard = [][]bot.KeyboardButton{
	bot.NewKeyboardButtons(conf.CommandCancel),
}

// initialization
func init() {
	launched = time.Now()

	// read variables from config file
	if config, err := helper.GetConfig(); err == nil {
		apiToken = config.ApiToken
		availableIds = config.AvailableIds
		monitorInterval = config.MonitorInterval
		if monitorInterval <= 0 {
			monitorInterval = conf.DefaultMonitorIntervalSeconds
		}
		isVerbose = config.IsVerbose

		// image width * height
		imageWidth = config.ImageWidth
		if imageWidth < conf.MinImageWidth {
			imageWidth = conf.MinImageWidth
		}
		imageHeight = config.ImageHeight
		if imageHeight < conf.MinImageHeight {
			imageHeight = conf.MinImageHeight
		}

		// other camera params
		cameraParams = config.CameraParams

		// maintenance
		isInMaintenance = config.IsInMaintenance
		maintenanceMessage = config.MaintenanceMessage
		if len(maintenanceMessage) <= 0 {
			maintenanceMessage = conf.DefaultMaintenanceMessage
		}

		// initialize session variables
		sessions := make(map[string]Session)
		for _, v := range availableIds {
			sessions[v] = Session{
				UserId:        v,
				CurrentStatus: StatusWaiting,
				LastUpdateId:  -1,
			}
		}
		pool = SessionPool{
			Sessions: sessions,
		}

		// channels
		captureChannel = make(chan CaptureRequest, NumQueue)

		// loggly
		if config.LogglyToken != "" {
			logger = loggly.New(config.LogglyToken)
		} else {
			logger = nil
		}

		// local database
		db = helper.OpenDb()
	} else {
		panic(err.Error())
	}
}

// check if given Telegram id is available
func isAvailableId(id string) bool {
	for _, v := range availableIds {
		if v == id {
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

https://github.com/meinside/telegram-bot-rpi-camera
`,
		conf.CommandCapture,
		conf.CommandStatus,
		conf.CommandHelp,
	)
}

// for showing current status of this bot
func getStatus() string {
	return fmt.Sprintf("Uptime: %s\nMemory Usage: %s", helper.GetUptime(launched), helper.GetMemoryUsage())
}

// process incoming update from Telegram
func processUpdate(b *bot.Bot, update bot.Update) bool {
	// check username
	var userId string
	if update.Message.From.Username == nil {
		logError(fmt.Sprintf("Message - User not allowed (has no username): %s", update.Message.From.FirstName))
		return false
	}
	userId = *update.Message.From.Username
	if !isAvailableId(userId) {
		logError(fmt.Sprintf("Message - Id not allowed: %s", userId))
		return false
	}

	// process result
	result := false

	pool.Lock()
	if session, exists := pool.Sessions[userId]; exists {
		// XXX - for skipping duplicated update
		// (sometimes same update is retrieved again and again due to Telegram's API error)
		if session.LastUpdateId != update.UpdateId {
			// save last update id
			pool.Sessions[userId] = Session{
				UserId:        session.UserId,
				CurrentStatus: session.CurrentStatus,
				LastUpdateId:  update.UpdateId,
			}

			// text from message
			var txt string
			if update.Message.HasText() {
				txt = *update.Message.Text
			} else {
				txt = ""
			}

			var message, cmd string
			var options map[string]interface{} = map[string]interface{}{
				"reply_markup": bot.ReplyKeyboardMarkup{
					Keyboard:       allKeyboards,
					ResizeKeyboard: true,
				},
				"parse_mode": bot.ParseModeMarkdown,
			}

			switch session.CurrentStatus {
			case StatusWaiting:
				switch {
				// start
				case strings.HasPrefix(txt, conf.CommandStart):
					message = conf.MessageDefault
					cmd = conf.CommandStart
				// capture
				case strings.HasPrefix(txt, conf.CommandCapture):
					message = ""
					cmd = conf.CommandCapture
				// status
				case strings.HasPrefix(txt, conf.CommandStatus):
					message = getStatus()
					cmd = conf.CommandStatus
				// help
				case strings.HasPrefix(txt, conf.CommandHelp):
					message = getHelp()
					cmd = conf.CommandHelp
				// fallback
				default:
					message = fmt.Sprintf("*%s*: %s", txt, conf.MessageUnknownCommand)
					cmd = "unknown"
				}
			}

			// log request
			logRequest(userId, cmd)

			if len(message) > 0 {
				// 'typing...'
				b.SendChatAction(update.Message.Chat.Id, bot.ChatActionTyping)

				// send message
				if sent := b.SendMessage(update.Message.Chat.Id, message, options); sent.Ok {
					result = true
				} else {
					logError(fmt.Sprintf("Failed to send message: %s", *sent.Description))
				}
			} else {
				if isInMaintenance {
					// send message
					if sent := b.SendMessage(update.Message.Chat.Id, maintenanceMessage, options); sent.Ok {
						result = true
					} else {
						logError(fmt.Sprintf("Failed to send maintenance message: %s", *sent.Description))
					}
				} else {
					// push to capture request channel
					captureChannel <- CaptureRequest{
						UserName:       *update.Message.From.Username,
						ChatId:         update.Message.Chat.Id,
						ImageWidth:     imageWidth,
						ImageHeight:    imageHeight,
						CameraParams:   cameraParams,
						MessageOptions: options,
					}
				}
			}
		} else {
			logError(fmt.Sprintf("Duplicated update id: %d", update.UpdateId))
		}
	} else {
		logError(fmt.Sprintf("Session does not exist for id: %s", userId))
	}
	pool.Unlock()

	return result
}

// process capture request
func processCaptureRequest(b *bot.Bot, request CaptureRequest) bool {
	// process result
	result := false

	cameraLock.Lock()
	defer cameraLock.Unlock()

	// 'typing...'
	b.SendChatAction(request.ChatId, bot.ChatActionTyping)

	// send photo
	if bytes, err := helper.CaptureRaspiStill(request.ImageWidth, request.ImageHeight, request.CameraParams); err == nil {
		// captured time
		caption := time.Now().Format("2006-01-02 (Mon) 15:04:05")
		request.MessageOptions["caption"] = caption

		// 'uploading photo...'
		b.SendChatAction(request.ChatId, bot.ChatActionUploadPhoto)

		// send photo
		if sent := b.SendPhoto(request.ChatId, bot.InputFileFromBytes(bytes), request.MessageOptions); sent.Ok {
			photo := sent.Result.LargestPhoto()

			db.SavePhoto(request.UserName, photo.FileId, caption)

			result = true
		} else {
			logError(fmt.Sprintf("Failed to send photo: %s", *sent.Description))
		}
	} else {
		message := fmt.Sprintf("Image capture failed: %s", err)

		logError(message)

		b.SendMessage(request.ChatId, message, request.MessageOptions)
	}

	return result
}

// process inline query
func processInlineQuery(b *bot.Bot, update bot.Update) bool {
	// check username
	var userId string
	if update.InlineQuery.From.Username == nil {
		logError(fmt.Sprintf("Inline Query - user not allowed (has no username): %s", update.Message.From.FirstName))
		return false
	}
	userId = *update.InlineQuery.From.Username
	if !isAvailableId(userId) {
		logError(fmt.Sprintf("Inline Query - id not allowed: %s", userId))
		return false
	}

	// retrieve cached photos,
	photos := db.GetPhotos(userId, NumLatestPhotos)

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
		if sent := b.AnswerInlineQuery(
			update.InlineQuery.Id,
			photoResults,
			nil,
		); sent.Ok {
			return true
		} else {
			logError(fmt.Sprintf("Failed to answer inline query: %s", *sent.Description))
		}
	} else {
		logError("No cached photos for inline query.")
	}

	return false
}

func main() {
	client := bot.NewClient(apiToken)
	client.Verbose = isVerbose

	// get info about this bot
	if me := client.GetMe(); me.Ok {
		logMessage(fmt.Sprintf("Starting bot: @%s (%s)\n", *me.Result.Username, me.Result.FirstName))

		// delete webhook (getting updates will not work when wehbook is set up)
		if unhooked := client.DeleteWebhook(); unhooked.Ok {
			// monitor request capture channel
			go func() {
				for {
					select {
					case request := <-captureChannel:
						// do capture and send response
						processCaptureRequest(client, request)
					}
				}
			}()

			// wait for new updates
			client.StartMonitoringUpdates(0, monitorInterval, func(b *bot.Bot, update bot.Update, err error) {
				if err == nil {
					if update.HasMessage() {
						processUpdate(b, update)
					} else if update.HasInlineQuery() {
						processInlineQuery(b, update)
					}
				} else {
					logError(fmt.Sprintf("Error while receiving update (%s)", err.Error()))
				}
			})
		} else {
			panic("Failed to delete webhook")
		}
	} else {
		panic("Failed to get info of the bot")
	}
}

func logMessage(message string) {
	log.Println(message)

	if logger != nil {
		logger.Log(LogglyLog{
			Application: AppName,
			Severity:    "Log",
			Message:     message,
		})
	}
}

func logError(message string) {
	log.Println(message)

	if logger != nil {
		logger.Log(LogglyLog{
			Application: AppName,
			Severity:    "Error",
			Message:     message,
		})
	}
}

func logRequest(username, cmd string) {
	if logger != nil {
		logger.Log(LogglyLog{
			Application: AppName,
			Severity:    "Verbose",
			Object: struct {
				Username string `json:"username"`
				Command  string `json:"command"`
			}{
				Username: username,
				Command:  cmd,
			},
		})
	}
}
