# Telegram Bot for Capturing Images with Raspberry Pi Camera Module

With this bot, you can capture images with camera module on your Raspberry Pi.

## 0. Prepare,

Install Go and generate your Telegram bot's API token.

## 1. Install and configure,

```bash
$ go get -d github.com/meinside/telegram-rpi-camera-bot
$ cd $GOPATH/src/github.com/meinside/telegram-rpi-camera-bot
$ cp config.json.sample config.json
$ vi config.json
```

and edit values to yours:

```json
{
  "available_ids": [
    "telegram_id_1",
    "telegram_id_2",
    "telegram_id_3"
  ],
  "monitor_interval": 3,
  "image_width": 1600,
  "image_height": 1200,
  "is_verbose": false,

  "api_token": "0123456789:abcdefghijklmnopqrstuvwyz-x-0a1b2c3d4e"
}
```

### Using Infisical

You can also use [Infisical](https://infisical.com/) for retrieving your bot api token:

```json
{
  "available_ids": [
    "telegram_id_1",
    "telegram_id_2",
    "telegram_id_3"
  ],
  "monitor_interval": 3,
  "image_width": 1600,
  "image_height": 1200,
  "is_verbose": false,

  "infisical": {
    "client_id": "012345-abcdefg-987654321",
    "client_secret": "aAbBcCdDeEfFgG0123456789xyzwXYZW",

    "project_id": "012345abcdefg",
    "environment": "dev",
    "secret_type": "shared",

    "api_token_key_path": "/path/to/your/KEY_TO_API_TOKEN"
  }
}
```

## 2. Build,

### A. build manually,

```bash
$ go build
```

### B. or build with docker-compose

#### a. Raspberry Pi 3B, 3B+

```bash
$ docker-compose build
```

#### b. Raspberry Pi 2

```bash
$ docker-compose build --build-arg RPI=raspberry-pi2
```

#### c. Raspberry Pi B / Zero

```bash
$ docker-compose build --build-arg RPI=raspberry-pi
```

## 3. And Run

### A. run manually,

```bash
$ ./telegram-rpi-camera-bot
```

### B. run as a service with systemd,

```bash
$ sudo cp systemd/telegram-rpi-camera-bot.service /etc/systemd/system/
$ sudo vi /etc/systemd/system/telegram-rpi-camera-bot.service
```

and edit **User**, **Group**, **WorkingDirectory** and **ExecStart** values.

It will launch automatically on boot with:

```bash
$ sudo systemctl enable telegram-rpi-camera-bot.service
```

and will start with:

```bash
$ sudo systemctl start telegram-rpi-camera-bot.service
```

### C. or run with docker-compose

```bash
$ docker-compose up -d
```

## 998. Trouble shooting

TODO

## 999. License

MIT

