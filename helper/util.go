package helper

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	LibCameraStillBin               = "/usr/bin/libcamera-still"
	LibCameraStillRunTimeoutSeconds = 10
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
	IsVerbose          bool                   `json:"is_verbose"`
}

// GetConfig reads config
func GetConfig() (config Config, err error) {
	var execFilepath string
	if execFilepath, err = os.Executable(); err == nil {
		var file []byte
		if file, err = os.ReadFile(filepath.Join(filepath.Dir(execFilepath), ConfigFilename)); err == nil {
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

// CaptureStillImage captures an image with `raspistill`.
func CaptureStillImage(libcameraStillBinPath string, width, height int, cameraParams map[string]interface{}) (result []byte, err error) {
	// command line arguments
	args := []string{
		"--width", strconv.Itoa(width),
		"--height", strconv.Itoa(height),
		"--encoding", "jpg",
		"--output", "-", // output to stdout
	}
	for k, v := range cameraParams {
		args = append(args, k)
		if v != nil {
			args = append(args, fmt.Sprintf("%v", v))
		}
	}

	// execute command with timeout,
	cmd := exec.Command(libcameraStillBinPath, args...)
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	err = cmd.Start()
	if err == nil {
		done := make(chan error)
		go func() { done <- cmd.Wait() }()
		timeout := time.After(LibCameraStillRunTimeoutSeconds * time.Second)

		// and get its standard output
		select {
		case <-timeout:
			err = cmd.Process.Kill()
			if err == nil {
				err = fmt.Errorf("Command timed out: %s", libcameraStillBinPath)
			} else {
				err = fmt.Errorf("Command timed out, but failed to kill process: %s", libcameraStillBinPath)
			}
		case err = <-done:
			if err == nil {
				return buffer.Bytes(), nil
			} else {
				err = fmt.Errorf("Error running %s: %s", libcameraStillBinPath, err)
			}
		}
	}

	return nil, err
}
