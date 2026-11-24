package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"SystemController/solarctl"
)

// The solar pump settings start from solarctl.DefaultConfig, then the
// -solar... flags, then the config file if it exists. The tuning page
// (web/solar_tuning.html) changes them while the controller runs and can
// write them back to the file.

var solarConfigFile string

func init() {
	flag.StringVar(&solarConfigFile, "solarConfigFile", "/etc/SystemController/solarpump.json",
		"File holding the solar pump controller settings, written by the solar tuning page")
}

// solarSettings holds the settings in use. ManageSolarPump picks up a change
// before its next step.
var solarSettings struct {
	once    sync.Once
	mu      sync.Mutex
	cfg     solarctl.Config
	changed bool
}

// currentSolarConfig returns the settings in use, loading the config file the
// first time.
func currentSolarConfig() solarctl.Config {
	solarSettings.once.Do(loadSolarConfig)
	solarSettings.mu.Lock()
	defer solarSettings.mu.Unlock()
	return solarSettings.cfg
}

// setSolarConfig makes cfg the settings in use; the controller applies it on
// its next step.
func setSolarConfig(cfg solarctl.Config) {
	solarSettings.once.Do(loadSolarConfig)
	solarSettings.mu.Lock()
	defer solarSettings.mu.Unlock()
	solarSettings.cfg, solarSettings.changed = cfg, true
}

// takeSolarConfigChange returns the new settings if they changed since the last call.
func takeSolarConfigChange() (solarctl.Config, bool) {
	solarSettings.mu.Lock()
	defer solarSettings.mu.Unlock()
	changed := solarSettings.changed
	solarSettings.changed = false
	return solarSettings.cfg, changed
}

func loadSolarConfig() {
	solarSettings.cfg = solarConfig // defaults overridden by any -solar... flags
	cfg, err := readSolarConfigFile(solarConfig)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		log.Printf("Solar pump config file %s not found, using the built in settings", solarConfigFile)
		return
	case err != nil:
		log.Printf("Solar pump config file - %s. Using the built in settings", err)
		return
	}
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "solarMinSpeed", "solarTargetRise", "solarTankMax", "solarTankResume":
			log.Printf("Solar pump: -%s is ignored, the value in %s is used", f.Name, solarConfigFile)
		}
	})
	solarSettings.cfg = cfg
	log.Printf("Solar pump settings loaded from %s", solarConfigFile)
}

// readSolarConfigFile reads the config file over base, so settings missing
// from the file keep their value in base.
func readSolarConfigFile(base solarctl.Config) (solarctl.Config, error) {
	b, err := os.ReadFile(solarConfigFile)
	if err != nil {
		return base, err
	}
	cfg := base
	if err := json.Unmarshal(b, &cfg); err != nil {
		return base, fmt.Errorf("%s: %w", solarConfigFile, err)
	}
	if err := cfg.Validate(); err != nil {
		return base, fmt.Errorf("%s: %w", solarConfigFile, err)
	}
	return cfg, nil
}

// writeSolarConfigFile saves cfg, replacing the file in one step so a power
// cut never leaves it half written.
func writeSolarConfigFile(cfg solarctl.Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(solarConfigFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".solarpump-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // fails harmlessly once renamed
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), solarConfigFile)
}

// solarConfigReply is the JSON returned by /solarPump/config.
type solarConfigReply struct {
	Current  solarctl.Config  `json:"current"`
	Saved    *solarctl.Config `json:"saved"` // nil if there is no readable config file
	Defaults solarctl.Config  `json:"defaults"`
	File     string           `json:"file"`
	FileErr  string           `json:"fileError,omitempty"`
	Message  string           `json:"message,omitempty"`
}

func sendSolarConfig(w http.ResponseWriter, message string) {
	reply := solarConfigReply{
		Current:  currentSolarConfig(),
		Defaults: solarctl.DefaultConfig(),
		File:     solarConfigFile,
		Message:  message,
	}
	if saved, err := readSolarConfigFile(solarctl.DefaultConfig()); err == nil {
		reply.Saved = &saved
	} else if !errors.Is(err, fs.ErrNotExist) {
		reply.FileErr = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(reply); err != nil {
		log.Println("sendSolarConfig() - ", err)
	}
}

// getSolarConfig returns the solar pump settings in use, those in the config
// file and the defaults.
func getSolarConfig(w http.ResponseWriter, _ *http.Request) {
	sendSolarConfig(w, "")
}

// postSolarConfig applies new solar pump settings straight away. With
// ?save=true it also writes them to the config file.
func postSolarConfig(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil {
		http.Error(w, "Could not read the settings - "+err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := solarctl.DecodeConfig(b, currentSolarConfig())
	if err != nil {
		http.Error(w, "The settings are not valid - "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := cfg.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	setSolarConfig(cfg)
	msg := "Applied"
	if r.URL.Query().Get("save") == "true" {
		if err := writeSolarConfigFile(cfg); err != nil {
			log.Printf("Solar pump: saving %s - %s", solarConfigFile, err)
			http.Error(w, "Applied, but could not save to "+solarConfigFile+" - "+err.Error(), http.StatusInternalServerError)
			return
		}
		msg = "Applied and saved"
		log.Printf("Solar pump settings saved to %s", solarConfigFile)
	}
	sendSolarConfig(w, msg)
}
