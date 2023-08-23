package main

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

	"github.com/meinside/infisical-go"
	"github.com/meinside/infisical-go/helper"
)

const (
	// constants for config
	configFilename = "config.json"

	libCameraStillBin               = "/usr/bin/libcamera-still"
	libCameraStillRunTimeoutSeconds = 10
)

// struct for config file
type config struct {
	AvailableIds       []string               `json:"available_ids"`
	MonitorInterval    int                    `json:"monitor_interval"`
	ImageWidth         int                    `json:"image_width"`
	ImageHeight        int                    `json:"image_height"`
	CameraParams       map[string]interface{} `json:"camera_params"`
	IsInMaintenance    bool                   `json:"is_in_maintenance"`
	MaintenanceMessage string                 `json:"maintenance_message"`
	IsVerbose          bool                   `json:"is_verbose"`

	// Bot API Token,
	APIToken string `json:"api_token,omitempty"`

	// or Infisical settings
	Infisical *struct {
		// NOTE: When the workspace's E2EE setting is enabled, APIKey is essential for decryption
		E2EE   bool    `json:"e2ee,omitempty"`
		APIKey *string `json:"api_key,omitempty"`

		WorkspaceID string               `json:"workspace_id"`
		Token       string               `json:"token"`
		Environment string               `json:"environment"`
		SecretType  infisical.SecretType `json:"secret_type"`

		APITokenKeyPath string `json:"api_token_key_path"`
	} `json:"infisical,omitempty"`
}

// loadConfig reads config
func loadConfig() (conf config, err error) {
	var execFilepath string
	if execFilepath, err = os.Executable(); err == nil {
		var file []byte
		if file, err = os.ReadFile(filepath.Join(filepath.Dir(execFilepath), configFilename)); err == nil {
			var conf config
			if err = json.Unmarshal(file, &conf); err == nil {
				if conf.APIToken == "" && conf.Infisical != nil {
					var apiToken string

					// read bot api token from infisical
					if conf.Infisical.E2EE && conf.Infisical.APIKey != nil {
						apiToken, err = helper.E2EEValue(
							*conf.Infisical.APIKey,
							conf.Infisical.WorkspaceID,
							conf.Infisical.Token,
							conf.Infisical.Environment,
							conf.Infisical.SecretType,
							conf.Infisical.APITokenKeyPath,
						)
					} else {
						apiToken, err = helper.Value(
							conf.Infisical.WorkspaceID,
							conf.Infisical.Token,
							conf.Infisical.Environment,
							conf.Infisical.SecretType,
							conf.Infisical.APITokenKeyPath,
						)
					}

					conf.APIToken = apiToken
				}

				return conf, err
			}
		}
	}

	return config{}, err
}

// getUptime gets uptime of this bot in seconds
func getUptime(launched time.Time) (uptime string) {
	now := time.Now()
	gap := now.Sub(launched)

	uptimeSeconds := int(gap.Seconds())
	numDays := uptimeSeconds / (60 * 60 * 24)
	numHours := (uptimeSeconds % (60 * 60 * 24)) / (60 * 60)

	return fmt.Sprintf("*%d* day(s) *%d* hour(s)", numDays, numHours)
}

// getMemoryUsage gets memory usage
func getMemoryUsage() (usage string) {
	m := new(runtime.MemStats)
	runtime.ReadMemStats(m)

	return fmt.Sprintf("Sys: *%.1f MB*, Heap: *%.1f MB*", float32(m.Sys)/1024/1024, float32(m.HeapAlloc)/1024/1024)
}

// captureStillImage captures an image with `raspistill`.
func captureStillImage(libcameraStillBinPath string, width, height int, cameraParams map[string]interface{}) (result []byte, err error) {
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
		timeout := time.After(libCameraStillRunTimeoutSeconds * time.Second)

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
