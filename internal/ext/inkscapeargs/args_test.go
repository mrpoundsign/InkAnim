package inkscapeargs

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantReq    Request
		wantErr    bool
	}{
		{
			name: "no IDs, default mode",
			args: []string{"/tmp/input.svg"},
			wantReq: Request{
				Mode:      "editor",
				IDs:       nil,
				Params:    map[string]string{},
				InputPath: "/tmp/input.svg",
			},
			wantErr: false,
		},
		{
			name: "multiple IDs",
			args: []string{"--id=rect1", "--id=g2", "test.svg"},
			wantReq: Request{
				Mode:      "editor",
				IDs:       []string{"rect1", "g2"},
				Params:    map[string]string{},
				InputPath: "test.svg",
			},
			wantErr: false,
		},
		{
			name: "unknown params and custom mode",
			args: []string{"--mode=frame", "--frame=5", "--preview=true", "--id=p1", "doc.svg"},
			wantReq: Request{
				Mode: "frame",
				IDs:  []string{"p1"},
				Params: map[string]string{
					"frame":   "5",
					"preview": "true",
				},
				InputPath: "doc.svg",
			},
			wantErr: false,
		},
		{
			name: "paths with spaces",
			args: []string{"--id=obj1", "C:\\Users\\User Name\\My Drawings\\sample project.svg"},
			wantReq: Request{
				Mode:      "editor",
				IDs:       []string{"obj1"},
				Params:    map[string]string{},
				InputPath: "C:\\Users\\User Name\\My Drawings\\sample project.svg",
			},
			wantErr: false,
		},
		{
			name:    "missing path",
			args:    []string{"--id=obj1"},
			wantErr: true,
		},
		{
			name:    "empty args",
			args:    []string{},
			wantErr: true,
		},
		{
			name: "space separated flag and value",
			args: []string{"--id", "obj1", "--mode", "export", "/path/to/file.svg"},
			wantReq: Request{
				Mode:      "export",
				IDs:       []string{"obj1"},
				Params:    map[string]string{},
				InputPath: "/path/to/file.svg",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Mode != tt.wantReq.Mode {
				t.Errorf("Mode = %q, want %q", got.Mode, tt.wantReq.Mode)
			}
			if !reflect.DeepEqual(got.IDs, tt.wantReq.IDs) {
				t.Errorf("IDs = %v, want %v", got.IDs, tt.wantReq.IDs)
			}
			if !reflect.DeepEqual(got.Params, tt.wantReq.Params) {
				t.Errorf("Params = %v, want %v", got.Params, tt.wantReq.Params)
			}
			if got.InputPath != tt.wantReq.InputPath {
				t.Errorf("InputPath = %q, want %q", got.InputPath, tt.wantReq.InputPath)
			}
		})
	}
}
