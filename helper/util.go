package helper

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

const (
	// constants for config
	ConfigFilename = "../config.json"

	// absolute path of raspistill
	RaspiStillBin = "/usr/bin/raspistill"
)

// struct for config file
type Config struct {
	ApiToken        string                 `json:"api_token"`
	AvailableIds    []string               `json:"available_ids"`
	MonitorInterval int                    `json:"monitor_interval"`
	ImageWidth      int                    `json:"image_width"`
	ImageHeight     int                    `json:"image_height"`
	CameraParams    map[string]interface{} `json:"camera_params"`
	IsVerbose       bool                   `json:"is_verbose"`
}

// Read config
func GetConfig() (config Config, err error) {
	_, filename, _, _ := runtime.Caller(0) // = __FILE__

	if file, err := ioutil.ReadFile(filepath.Join(path.Dir(filename), ConfigFilename)); err == nil {
		var conf Config
		if err := json.Unmarshal(file, &conf); err == nil {
			return conf, nil
		} else {
			return Config{}, err
		}
	} else {
		return Config{}, err
	}
}

// get uptime of this bot in seconds
func GetUptime(launched time.Time) (uptime string) {
	now := time.Now()
	gap := now.Sub(launched)

	uptimeSeconds := int(gap.Seconds())
	numDays := uptimeSeconds / (60 * 60 * 24)
	numHours := (uptimeSeconds % (60 * 60 * 24)) / (60 * 60)

	return fmt.Sprintf("*%d* day(s) *%d* hour(s)", numDays, numHours)
}

// get memory usage
func GetMemoryUsage() (usage string) {
	m := new(runtime.MemStats)
	runtime.ReadMemStats(m)

	return fmt.Sprintf("Sys: *%.1f MB*, Heap: *%.1f MB*", float32(m.Sys)/1024/1024, float32(m.HeapAlloc)/1024/1024)
}

// capture an image with given width, height, and other parameters
// return the captured image's filepath (for deleting it after use)
func CaptureRaspiStill(directory string, width, height int, cameraParams map[string]interface{}) (filepath string, err error) {
	// filepath
	filepath = fmt.Sprintf("%s/captured_%d.jpg", directory, time.Now().UnixNano()/int64(time.Millisecond))

	// command line arguments
	args := []string{
		"-w", strconv.Itoa(width),
		"-h", strconv.Itoa(height),
		"-o", filepath,
	}
	for k, v := range cameraParams {
		args = append(args, k)
		if v != nil {
			args = append(args, fmt.Sprintf("%v", v))
		}
	}

	// execute command
	if bytes, err := exec.Command(RaspiStillBin, args...).CombinedOutput(); err != nil {
		log.Printf("*** Error running %s: %s\n", RaspiStillBin, string(bytes))
		return "", err
	} else {
		return filepath, nil
	}
}
