package helper

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

const (
	// constants for config
	ConfigFilename = "config.json"

	RaspiStillBin    = "/usr/bin/raspistill"
	FfmpegBinDefault = "/usr/local/bin/ffmpeg"
)

// struct for config file
type Config struct {
	ApiToken           string                 `json:"api_token"`
	AvailableIds       []string               `json:"available_ids"`
	MonitorInterval    int                    `json:"monitor_interval"`
	ImageWidth         int                    `json:"image_width"`
	ImageHeight        int                    `json:"image_height"`
	CameraParams       map[string]interface{} `json:"camera_params"`
	IsInMaintenance    bool                   `json:"is_in_maintenance"`
	MaintenanceMessage string                 `json:"maintenance_message"`
	LogglyToken        string                 `json:"loggly_token,omitempty"`
	IsVerbose          bool                   `json:"is_verbose"`

	UseFfmpeg     bool   `json:"use_ffmpeg,omitempty"`
	FfmpegBinPath string `json:"ffmpeg_bin_path,omitempty"`
}

// GetConfig reads config
func GetConfig() (config Config, err error) {
	var execFilepath string
	if execFilepath, err = os.Executable(); err == nil {
		var file []byte
		if file, err = ioutil.ReadFile(filepath.Join(filepath.Dir(execFilepath), ConfigFilename)); err == nil {
			var conf Config
			if err = json.Unmarshal(file, &conf); err == nil {
				return conf, nil
			}
		}
	}

	return Config{}, err
}

// GetUptime gets uptime of this bot in seconds
func GetUptime(launched time.Time) (uptime string) {
	now := time.Now()
	gap := now.Sub(launched)

	uptimeSeconds := int(gap.Seconds())
	numDays := uptimeSeconds / (60 * 60 * 24)
	numHours := (uptimeSeconds % (60 * 60 * 24)) / (60 * 60)

	return fmt.Sprintf("*%d* day(s) *%d* hour(s)", numDays, numHours)
}

// GetMemoryUsage gets memory usage
func GetMemoryUsage() (usage string) {
	m := new(runtime.MemStats)
	runtime.ReadMemStats(m)

	return fmt.Sprintf("Sys: *%.1f MB*, Heap: *%.1f MB*", float32(m.Sys)/1024/1024, float32(m.HeapAlloc)/1024/1024)
}

// CaptureRaspiStill captures an image with `raspistill`.
func CaptureRaspiStill(raspistillBinPath string, width, height int, cameraParams map[string]interface{}) (bytes []byte, err error) {
	// command line arguments
	args := []string{
		"-w", strconv.Itoa(width),
		"-h", strconv.Itoa(height),
		"-o", "-", // output to stdout
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
		return []byte{}, err
	} else {
		return bytes, nil
	}
}

// CaptureFfmpeg captures an image with `ffmpeg.`
//
// https://gist.github.com/moritzmhmk/48e5ed9c4baa5557422f16983900ca95#ffmpeg
func CaptureFfmpeg(ffmpegBinPath string, width, height int) (bytes []byte, err error) {
	// command line arguments
	args := []string{
		"-f", "video4linux2",
		"-input_format", "mjpeg",
		"-video_size", fmt.Sprintf("%dx%d", width, height),
		"-i", "/dev/video0",
		"-vframes", "1",
		"-f", "mjpeg",
		"-nostats",
		"-hide_banner",
		"-loglevel", "error",
		"-",
	}

	// execute command
	if bytes, err := exec.Command(ffmpegBinPath, args...).CombinedOutput(); err != nil {
		log.Printf("*** Error running %s: %s\n", ffmpegBinPath, string(bytes))
		return []byte{}, err
	} else {
		return bytes, nil
	}
}
