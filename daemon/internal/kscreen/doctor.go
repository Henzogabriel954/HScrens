package kscreen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type OutputInfo struct {
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
	Enabled   bool   `json:"enabled"`
}

type KScreenDocJSON struct {
	Outputs []OutputInfo `json:"outputs"`
}

// FindVirtualOutputName procura nas saídas do kscreen-doctor qual é o nome do output virtual (ex: "Virtual-1", "VIRTUAL-1").
func FindVirtualOutputName() string {
	outStr, err := GetOutputs()
	if err == nil {
		var doc KScreenDocJSON
		if err := json.Unmarshal([]byte(outStr), &doc); err == nil {
			for _, o := range doc.Outputs {
				if strings.HasPrefix(strings.ToLower(o.Name), "virtual") {
					return o.Name
				}
			}
		}
	}
	return "Virtual-1"
}

// EnableVirtualOutput calls kscreen-doctor to enable a virtual output with given parameters.
func EnableVirtualOutput(width, height int, position string) (string, error) {
	outputName := FindVirtualOutputName()
	modeArg := fmt.Sprintf("output.%s.mode.%dx%d@60", outputName, width, height)
	enableArg := fmt.Sprintf("output.%s.enable", outputName)
	posArg := fmt.Sprintf("output.%s.position.%s", outputName, position)

	cmd := exec.Command("kscreen-doctor", enableArg, modeArg, posArg)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		// Tenta apenas dar enable caso a sintaxe de mode/pos falhe
		cmd2 := exec.Command("kscreen-doctor", enableArg)
		var out2 bytes.Buffer
		cmd2.Stdout = &out2
		cmd2.Stderr = &out2
		if err2 := cmd2.Run(); err2 == nil {
			return out2.String(), nil
		}
		return out.String(), fmt.Errorf("failed to enable virtual output %s: %w, output: %s", outputName, err, out.String())
	}
	return out.String(), nil
}

// DisableVirtualOutput disables the virtual output.
func DisableVirtualOutput() (string, error) {
	outputName := FindVirtualOutputName()
	cmd := exec.Command("kscreen-doctor", fmt.Sprintf("output.%s.disable", outputName))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return out.String(), fmt.Errorf("failed to disable virtual output: %w, output: %s", err, out.String())
	}
	return out.String(), nil
}

// GetOutputs returns the JSON string from kscreen-doctor -j
func GetOutputs() (string, error) {
	cmd := exec.Command("kscreen-doctor", "-j")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return out.String(), fmt.Errorf("failed to get outputs: %w, output: %s", err, out.String())
	}

	return out.String(), nil
}

// PollOutputs fica em loop consultando os outputs a cada 1 segundo.
func PollOutputs(ctx context.Context, onChange func(jsonOutputs string)) {
	var lastOutputs string
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			outputs, err := GetOutputs()
			if err == nil && outputs != lastOutputs {
				lastOutputs = outputs
				onChange(outputs)
			}
		}
	}
}
