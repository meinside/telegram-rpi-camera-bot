// telegram bot for using raspberry pi camera module
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	bot "github.com/meinside/telegram-bot-go"
)

const (
	githubPageURL = "https://github.com/meinside/telegram-rpi-camera-bot"
)

type status int16

// constants
const (
	statusWaiting status = iota

	numQueue        = 4
	numLatestPhotos = 20

	resizeKeyboard = true

	chatActionTimeout  = 5 * time.Second
	sendMessageTimeout = 10 * time.Second
	sendPhotoTimeout   = 30 * time.Second
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
	ChatID         any
	ImageWidth     int
	ImageHeight    int
	CameraParams   map[string]any
	MessageOptions map[string]any
}

// variables
var (
	apiToken                string
	monitorInterval         int
	isVerbose               bool
	availableIds            []string
	imageWidth, imageHeight int
	cameraParams            map[string]any
	isInMaintenance         bool
	maintenanceMessage      string
	pool                    _sessionPool
	captureChannel          chan _captureRequest
	launched                time.Time
	db                      *Database
)

// keyboards
var allKeyboards = [][]bot.KeyboardButton{
	bot.NewKeyboardButtons(commandCapture),
	bot.NewKeyboardButtons(commandStatus, commandPrivacy, commandHelp),
}

// loggers
var (
	_stdout = log.New(os.Stdout, "", log.LstdFlags)
	_stderr = log.New(os.Stderr, "", log.LstdFlags)
)

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
		imageWidth = max(config.ImageWidth, minImageWidth)
		imageHeight = max(config.ImageHeight, minImageHeight)

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

	return slices.Contains(availableIds, *id)
}

// for showing help message
func getHelp() string {
	return fmt.Sprintf(`
Following commands are supported:

*For Raspberry Pi Camera Module*

%s : capture a still image with *raspistill*

*Others*

%s : show this bot's status
%s : show this bot's privacy policy
%s : show this help message

%s
`,
		commandCapture,

		commandStatus,
		commandPrivacy,
		commandHelp,

		githubPageURL,
	)
}

// for showing privacy policy
func getPrivacyPolicy() string {
	return fmt.Sprintf(`
Privacy Policy:

%s/raw/master/PRIVACY.md
`, githubPageURL)
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
					// privacy
				case strings.HasPrefix(txt, commandPrivacy):
					msg = getPrivacyPolicy()
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
				chatActionCtx, cancel := context.WithTimeout(context.Background(), chatActionTimeout)
				defer cancel()
				_, _ = b.SendChatAction(chatActionCtx, message.Chat.ID, bot.ChatActionTyping, nil)

				// send message
				sendMessageCtx, cancel := context.WithTimeout(context.Background(), sendMessageTimeout)
				defer cancel()
				if sent, _ := b.SendMessage(sendMessageCtx, message.Chat.ID, msg, options); sent.OK {
					result = true
				} else {
					logError("failed to send message: %s", *sent.Description)
				}
			} else {
				if isInMaintenance {
					// send message
					sendMessageCtx, cancel := context.WithTimeout(context.Background(), sendMessageTimeout)
					defer cancel()
					if sent, _ := b.SendMessage(sendMessageCtx, message.Chat.ID, maintenanceMessage, options); sent.OK {
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
	chatActionCtx, cancel := context.WithTimeout(context.Background(), chatActionTimeout)
	defer cancel()
	_, _ = b.SendChatAction(chatActionCtx, request.ChatID, bot.ChatActionTyping, nil)

	// send photo
	if bytes, err := captureStillImage(libCameraStillBin, request.ImageWidth, request.ImageHeight, request.CameraParams); err == nil {
		// captured time
		caption := time.Now().Format("2006-01-02 (Mon) 15:04:05")
		request.MessageOptions["caption"] = caption

		// 'uploading photo...'
		chatActionCtx, cancel := context.WithTimeout(context.Background(), chatActionTimeout)
		defer cancel()
		_, _ = b.SendChatAction(chatActionCtx, request.ChatID, bot.ChatActionUploadPhoto, nil)

		// send photo
		sendPhotoCtx, cancel := context.WithTimeout(context.Background(), sendPhotoTimeout)
		defer cancel()
		if sent, _ := b.SendPhoto(sendPhotoCtx, request.ChatID, bot.NewInputFileFromBytes(bytes), request.MessageOptions); sent.OK {
			photo := sent.Result.LargestPhoto()

			db.savePhoto(request.UserName, photo.FileID, caption)

			result = true
		} else {
			msg := fmt.Sprintf("Failed to send photo: %s", *sent.Description)

			logError("%s", msg)

			// send error message
			sendMessageCtx, cancel := context.WithTimeout(context.Background(), sendMessageTimeout)
			defer cancel()
			_, _ = b.SendMessage(sendMessageCtx, request.ChatID, msg, nil)
		}
	} else {
		message := fmt.Sprintf("Image capture failed: %s", err)

		logError("%s", message)

		sendMessageCtx, cancel := context.WithTimeout(context.Background(), sendMessageTimeout)
		defer cancel()
		_, _ = b.SendMessage(sendMessageCtx, request.ChatID, message, request.MessageOptions)
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
		photoResults := []any{}

		// build up inline query results with cached photos,
		for _, photo := range photos {
			caption := photo.Caption

			if newPhoto, id := bot.NewInlineQueryResultCachedPhoto(photo.FileId); id != nil {
				newPhoto.Caption = &caption

				photoResults = append(photoResults, newPhoto)
			}
		}

		// then answer inline query
		answerQueryCtx, cancel := context.WithTimeout(context.Background(), sendMessageTimeout)
		defer cancel()
		sent, _ := b.AnswerInlineQuery(
			answerQueryCtx,
			inlineQuery.ID,
			photoResults,
			nil,
		)

		if sent.OK {
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
	getMeCtx, cancel := context.WithTimeout(context.Background(), sendMessageTimeout)
	defer cancel()
	if me, _ := client.GetMe(getMeCtx); me.OK {
		logMessage("starting bot: @%s (%s)", *me.Result.Username, me.Result.FirstName)

		// delete webhook (getting updates will not work when wehbook is set up)
		deleteWebhookCtx, cancel := context.WithTimeout(context.Background(), sendMessageTimeout)
		defer cancel()
		if unhooked, _ := client.DeleteWebhook(deleteWebhookCtx, false); unhooked.OK {
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

func logMessage(format string, a ...any) {
	_stdout.Printf(format, a...)
}

func logError(format string, a ...any) {
	_stderr.Printf(format, a...)
}
