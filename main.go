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

	"github.com/meinside/telegram-bot-rpi-camera/conf"
	"github.com/meinside/telegram-bot-rpi-camera/helper"
)

type Status int16

const (
	StatusWaiting Status = iota
)

const (
	TempDir = "/var/tmp" // 'tmpfs /var/tmp tmpfs nodev,nosuid,size=10M 0 0' in /etc/fstab

	NumQueue = 4
)

type Session struct {
	UserId        string
	CurrentStatus Status
}

// session pool for storing individual statuses
type SessionPool struct {
	Sessions map[string]Session
	sync.Mutex
}

// for making sure the camera is not used simultaneously
var cameraLock sync.Mutex

type CaptureRequest struct {
	ChatId         interface{}
	Directory      string
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
			}
		}
		pool = SessionPool{
			Sessions: sessions,
		}

		// channels
		captureChannel = make(chan CaptureRequest, NumQueue)
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
	return `
Following commands are supported:

*For Raspberry Pi Camera Module*

/capture : capture an still image with *raspistill*

*Others*

/status : show this bot's status
/help : show this help message
`
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
		log.Printf("*** Not allowed (no user name): %s\n", *update.Message.From.FirstName)
		return false
	}
	userId = *update.Message.From.Username
	if !isAvailableId(userId) {
		log.Printf("*** Id not allowed: %s\n", userId)
		return false
	}

	// process result
	result := false

	pool.Lock()
	if session, exists := pool.Sessions[userId]; exists {
		// text from message
		var txt string
		if update.Message.HasText() {
			txt = *update.Message.Text
		} else {
			txt = ""
		}

		var message string
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
			// capture
			case strings.HasPrefix(txt, conf.CommandCapture):
				message = ""
			// status
			case strings.HasPrefix(txt, conf.CommandStatus):
				message = getStatus()
			// help
			case strings.HasPrefix(txt, conf.CommandHelp):
				message = getHelp()
			// fallback
			default:
				message = fmt.Sprintf("*%s*: %s", txt, conf.MessageUnknownCommand)
			}
		}

		if len(message) > 0 {
			// 'typing...'
			b.SendChatAction(update.Message.Chat.Id, bot.ChatActionTyping)

			// send message
			if sent := b.SendMessage(update.Message.Chat.Id, &message, options); sent.Ok {
				result = true
			} else {
				log.Printf("*** Failed to send message: %s\n", *sent.Description)
			}
		} else {
			if isInMaintenance {
				// send message
				if sent := b.SendMessage(update.Message.Chat.Id, &maintenanceMessage, options); sent.Ok {
					result = true
				} else {
					log.Printf("*** Failed to send message: %s\n", *sent.Description)
				}
			} else {
				// push to capture request channel
				captureChannel <- CaptureRequest{
					ChatId:         update.Message.Chat.Id,
					Directory:      TempDir,
					ImageWidth:     imageWidth,
					ImageHeight:    imageHeight,
					CameraParams:   cameraParams,
					MessageOptions: options,
				}
			}
		}
	} else {
		log.Printf("*** Session does not exist for id: %s\n", userId)
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
	if filepath, err := helper.CaptureRaspiStill(request.Directory, request.ImageWidth, request.ImageHeight, request.CameraParams); err == nil {
		// 'uploading photo...'
		b.SendChatAction(request.ChatId, bot.ChatActionUploadPhoto)

		// send photo
		if sent := b.SendPhoto(request.ChatId, &filepath, request.MessageOptions); sent.Ok {
			if err := os.Remove(filepath); err != nil {
				log.Printf("*** Failed to delete temp file: %s\n", err)
			}
			result = true
		} else {
			log.Printf("*** Failed to send photo: %s\n", *sent.Description)
		}
	} else {
		log.Printf("*** Image capture failed: %s\n", err)
	}

	return result
}

func main() {
	client := bot.NewClient(apiToken)
	client.Verbose = isVerbose

	// get info about this bot
	if me := client.GetMe(); me.Ok {
		log.Printf("Launching bot: @%s (%s)\n", *me.Result.Username, *me.Result.FirstName)

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
					if update.Message != nil {
						processUpdate(b, update)
					}
				} else {
					log.Printf("*** Error while receiving update (%s)\n", err.Error())
				}
			})
		} else {
			panic("Failed to delete webhook")
		}
	} else {
		panic("Failed to get info of the bot")
	}
}
