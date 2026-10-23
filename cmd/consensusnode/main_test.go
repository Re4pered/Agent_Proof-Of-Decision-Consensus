package main

import (
	"strings"
	"testing"
)

func TestValidatePoADStartupConfig(t *testing.T) {
	tests := []struct {
		name          string
		required      bool
		registryPath  string
		registrySHA   string
		manifestPath  string
		manifestSHA   string
		wantErr       string
	}{
		{
			name: "PoAD disabled without registry",
		},
		{
			name:         "optional registry validation",
			registryPath: "registry.json",
			registrySHA:  "registry-digest",
		},
		{
			name:         "required validation with registry and manifest",
			required:     true,
			registryPath: "registry.json",
			registrySHA:  "registry-digest",
			manifestPath: "intents.jsonl",
			manifestSHA:  "manifest-digest",
		},
		{
			name:         "required without registry",
			required:     true,
			manifestPath: "intents.jsonl",
			manifestSHA:  "manifest-digest",
			wantErr:      "-poad-required needs",
		},
		{
			name:         "manifest without required enforcement",
			registryPath: "registry.json",
			registrySHA:  "registry-digest",
			manifestPath: "intents.jsonl",
			manifestSHA:  "manifest-digest",
			wantErr:      "-poad-intent-manifest requires -poad-required",
		},
		{
			name:         "registry path without digest",
			registryPath: "registry.json",
			wantErr:      "-poad-registry and -poad-registry-sha256",
		},
		{
			name:        "registry digest without path",
			registrySHA: "registry-digest",
			wantErr:     "-poad-registry and -poad-registry-sha256",
		},
		{
			name:        "manifest path without digest",
			required:    true,
			registryPath: "registry.json",
			registrySHA:  "registry-digest",
			manifestPath: "intents.jsonl",
			wantErr:      "-poad-intent-manifest and -poad-intent-manifest-sha256",
		},
		{
			name:         "manifest digest without path",
			manifestSHA:  "manifest-digest",
			wantErr:      "-poad-intent-manifest and -poad-intent-manifest-sha256",
		},
		{
			name:        "manifest without trusted registry",
			required:    true,
			manifestPath: "intents.jsonl",
			manifestSHA:  "manifest-digest",
			wantErr:      "-poad-required needs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePoADStartupConfig(
				tt.required,
				tt.registryPath,
				tt.registrySHA,
				tt.manifestPath,
				tt.manifestSHA,
			)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid configuration, got error: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
