# Telegram Bot for Capturing Images with Raspberry Pi Camera Module

With this bot, you can capture images with camera module on your Raspberry Pi.

## 0. Prepare

Install Go and generate your Telegram bot's API token.

## 1. Install and configure

```bash
$ git clone https://github.com/meinside/telegram-bot-rpi-camera.git
$ cd telegram-bot-rpi-camera
$ cp config.json.sample config.json
$ vi config.json
```

and edit values to yours:

```json
{
	"api_token": "0123456789:abcdefghijklmnopqrstuvwyz-x-0a1b2c3d4e",
	"available_ids": [
		"telegram_id_1",
		"telegram_id_2",
		"telegram_id_3"
	],
	"monitor_interval": 3,
	"image_width": 1600,
	"image_height": 1200,
	"is_verbose": false
}
```

## 2. Build and run

```bash
$ go build
```

and run it:

```bash
$ ./telegram-bot-rpi-camera
```

## 3. Run as a service

### systemd

```bash
$ sudo cp systemd/telegram-bot-rpi-camera.service /lib/systemd/system/
$ sudo vi /lib/systemd/system/telegram-bot-rpi-camera.service
```

and edit **User**, **Group**, **WorkingDirectory** and **ExecStart** values.

It will launch automatically on boot with:

```bash
$ sudo systemctl enable telegram-bot-rpi-camera.service
```

and will start with:

```bash
$ sudo systemctl start telegram-bot-rpi-camera.service
```

## 998. Trouble shooting

TODO

## 999. License

MIT

