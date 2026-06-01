package parquet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

type ParquetReader struct {
	path    string
	file    *os.File
	filters []LabelFilter
}

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level,omitempty"`
	Message   string    `json:"message"`
	Labels    map[string]string
}

func NewParquetReader(path string) (*ParquetReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return &ParquetReader{
		path: path,
		file: file,
	}, nil
}

func (r *ParquetReader) ApplyFilters(filters []LabelFilter) error {
	r.filters = filters
	return nil
}

func (r *ParquetReader) ReadAll() (data.Frames, error) {
	var entries []LogEntry
	buffer := make([]byte, 4096)
	remaining := make([]byte, 0)

	for {
		n, err := r.file.Read(buffer)
		if err != nil && err != io.EOF {
			return nil, err
		}
		if n == 0 {
			break
		}

		allData := append(remaining, buffer[:n]...)
		lines := bytes.Split(allData, []byte("\n"))
		
		// Save the last incomplete line
		if len(lines) > 0 {
			remaining = lines[len(lines)-1]
			lines = lines[:len(lines)-1]
		}

		for _, line := range lines {
			if len(line) == 0 {
				continue
			}

			var entry LogEntry
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}

			if entry.Timestamp.IsZero() {
				entry.Timestamp = time.Now()
			}

			if r.matchesFilters(entry) {
				entries = append(entries, entry)
			}
		}
	}

	// Process any remaining data
	if len(remaining) > 0 {
		var entry LogEntry
		if err := json.Unmarshal(remaining, &entry); err == nil {
			if entry.Timestamp.IsZero() {
				entry.Timestamp = time.Now()
			}
			if r.matchesFilters(entry) {
				entries = append(entries, entry)
			}
		}
	}

	return r.buildFrames(entries)
}

func (r *ParquetReader) matchesFilters(entry LogEntry) bool {
	if len(r.filters) == 0 {
		return true
	}

	for _, filter := range r.filters {
		value, exists := entry.Labels[filter.Key]
		if !exists {
			return false
		}

		switch filter.Operator {
		case "=", "==":
			found := false
			for _, v := range filter.Values {
				if value == v {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		case "!=", "<>":
			for _, v := range filter.Values {
				if value == v {
					return false
				}
			}
		case "=~":
			found := false
			for _, v := range filter.Values {
				if contains(value, v) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		case "!~":
			for _, v := range filter.Values {
				if contains(value, v) {
					return false
				}
			}
		default:
			return false
		}
	}

	return true
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (len(substr) == 0 || indexOf(s, substr) != -1)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func (r *ParquetReader) buildFrames(entries []LogEntry) (data.Frames, error) {
	if len(entries) == 0 {
		return data.Frames{}, nil
	}

	times := make([]time.Time, len(entries))
	messages := make([]string, len(entries))
	levels := make([]*string, len(entries))

	labelKeys := make(map[string]bool)
	for _, entry := range entries {
		for k := range entry.Labels {
			labelKeys[k] = true
		}
	}

	labelFields := make(map[string]*data.Field)
	for k := range labelKeys {
		labelFields[k] = data.NewField(k, nil, make([]*string, len(entries)))
	}

	for i, entry := range entries {
		times[i] = entry.Timestamp
		messages[i] = entry.Message
		if entry.Level != "" {
			levels[i] = &entry.Level
		}

		for k, field := range labelFields {
			if v, ok := entry.Labels[k]; ok {
				field.Set(i, &v)
			}
		}
	}

	frame := data.NewFrame(
		"parquet_logs",
		data.NewField("time", nil, times),
		data.NewField("message", nil, messages),
	)

	if hasValue(levels) {
		frame.Fields = append(frame.Fields, data.NewField("level", nil, levels))
	}

	for _, field := range labelFields {
		if hasValue(field.Values) {
			frame.Fields = append(frame.Fields, field)
		}
	}

	frame.SetMeta(&data.FrameMeta{
		Type: data.FrameTypeLogLines,
	})

	return data.Frames{frame}, nil
}

func hasValue(slice interface{}) bool {
	v := reflect.ValueOf(slice)
	if v.Kind() != reflect.Slice {
		return false
	}

	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		if elem.Kind() == reflect.Ptr {
			if !elem.IsNil() {
				return true
			}
		} else if !elem.IsZero() {
			return true
		}
	}
	return false
}

func (r *ParquetReader) Close() error {
	return r.file.Close()
}
