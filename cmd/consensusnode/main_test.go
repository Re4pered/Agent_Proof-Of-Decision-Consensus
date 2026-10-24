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


func TestValidatePoADProviderConfig(t *testing.T) {
	tests := []struct {
		name          string
		required      bool
		endpoint      string
		timeout       int
		requiredVotes int
		manifestPath  string
		wantErr       string
	}{
		{name: "disabled provider", timeout: 15, requiredVotes: 2},
		{name: "required without endpoint", required: true, timeout: 15, requiredVotes: 2, manifestPath: "intents.jsonl", wantErr: "-poad-required needs -poad-agent-supervisor-endpoint"},
		{name: "endpoint requires strict mode", endpoint: "http://localhost:8080/decisions", timeout: 15, requiredVotes: 2, manifestPath: "intents.jsonl", wantErr: "requires -poad-required"},
		{name: "endpoint requires manifest", required: true, endpoint: "http://localhost:8080/decisions", timeout: 15, requiredVotes: 2, wantErr: "requires -poad-intent-manifest"},
		{name: "timeout must be positive", required: true, endpoint: "http://localhost:8080/decisions", timeout: 0, requiredVotes: 2, manifestPath: "intents.jsonl", wantErr: "timeout must be positive"},
		{name: "votes must be positive", required: true, endpoint: "http://localhost:8080/decisions", timeout: 15, requiredVotes: 0, manifestPath: "intents.jsonl", wantErr: "-poad-required-votes must be positive"},
		{name: "valid strict provider", required: true, endpoint: "http://localhost:8080/decisions", timeout: 15, requiredVotes: 2, manifestPath: "intents.jsonl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePoADProviderConfig(tt.required, tt.endpoint, tt.timeout, tt.requiredVotes, tt.manifestPath)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid configuration, got error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
