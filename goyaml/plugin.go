// Package goyaml adapts the SnakeYAML shared parser library to go-yaml.
package goyaml

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"
	"olympos.io/encoding/edn"
)

type library struct {
	version string
	parse   func([]byte) ([]byte, error)
}

// Version is both the adapter release and its required native library version.
const Version = "0.1.0"

// Option configures a SnakeYAML parser.
type Option func(*Plugin)

// WithInstall controls downloading the pinned library when it is missing.
// Installation is enabled by default and never replaces an existing library.
func WithInstall(enabled bool) Option {
	return func(p *Plugin) { p.install = enabled }
}

// Plugin implements yaml.ParserPlugin using the ABI v2 shared library.
// Its library is loaded on the first Parse call and stays loaded for its lifetime.
type Plugin struct {
	once      sync.Once
	lib       *library
	err       error
	install   bool
	installer *installer
}

var _ yaml.ParserPlugin = (*Plugin)(nil)

// New creates a parser that locates its library through YAMLSTAR_LIBRARY_PATH
// or the standard plugin directories. Importing the package has no side effects.
func New(options ...Option) *Plugin {
	p := &Plugin{install: true}
	for _, option := range options {
		option(p)
	}
	return p
}

func (p *Plugin) load() (*library, error) {
	p.once.Do(func() {
		installMu.Lock()
		defer installMu.Unlock()
		p.lib, p.err = loadLibrary()
		if errors.Is(p.err, errLibraryNotFound) && p.install {
			download := p.installer
			if download == nil {
				download = newInstaller()
			}
			if p.err = download.install(searchDirectories()); p.err == nil {
				p.lib, p.err = loadLibrary()
			}
		}
	})
	return p.lib, p.err
}

// Register enables yaml-parser=snakeyaml without changing the native default.
// Registration performs no library loading or network requests.
// Call it during startup, before parsing configuration.
func Register() error {
	return yaml.RegisterPlugin(yaml.PluginRegistration{
		API: "yaml-parser", Name: "snakeyaml", Version: Version,
		Factory: func(cfg map[string]any) (any, error) {
			install := true
			for key, value := range cfg {
				if key != "install" {
					return nil, fmt.Errorf("snakeyaml: unknown option %q", key)
				}
				var ok bool
				if install, ok = value.(bool); !ok {
					return nil, errors.New("snakeyaml: install must be a boolean")
				}
			}
			return New(WithInstall(install)), nil
		},
	})
}

// Parse returns a complete YAML event stream. Concurrent calls are supported.
func (p *Plugin) Parse(input []byte) ([]yaml.PluginEvent, error) {
	if !utf8.Valid(input) {
		return nil, errors.New("snakeyaml input is not UTF-8")
	}
	lib, err := p.load()
	if err != nil {
		return nil, err
	}
	data, err := lib.parse(input)
	if err != nil {
		return nil, err
	}
	return decodeEvents(data)
}

type wireMark struct {
	Line   int `edn:"line"`
	Column int `edn:"column"`
	Index  int `edn:"index"`
}

func pluginMark(mark *wireMark) yaml.Mark {
	if mark == nil {
		return yaml.Mark{}
	}
	return yaml.Mark{Line: mark.Line + 1, Column: mark.Column + 1, Index: mark.Index}
}

type wireEvent struct {
	StartMark *wireMark         `edn:"start-mark"`
	EndMark   *wireMark         `edn:"end-mark"`
	Tags      map[string]string `edn:"tags"`
	Event     string            `edn:"event"`
	Value     string            `edn:"value"`
	Style     string            `edn:"style"`
	Anchor    string            `edn:"anchor"`
	Tag       string            `edn:"tag"`
	Name      string            `edn:"name"`
	Version   string            `edn:"version"`
	Flow      bool              `edn:"flow"`
	Explicit  bool              `edn:"explicit"`
}

func decodeEvents(data []byte) ([]yaml.PluginEvent, error) {
	var source []wireEvent
	if err := edn.Unmarshal(data, &source); err != nil {
		return nil, fmt.Errorf("snakeyaml event data: %w", err)
	}
	events := make([]yaml.PluginEvent, len(source))
	for i, event := range source {
		switch event.Event {
		case "stream_start", "stream_end", "document_start", "document_end",
			"mapping_start", "mapping_end", "sequence_start", "sequence_end",
			"scalar", "alias":
		default:
			return nil, fmt.Errorf("unknown snakeyaml event %q", event.Event)
		}
		events[i] = yaml.PluginEvent{
			StartMark: pluginMark(event.StartMark), EndMark: pluginMark(event.EndMark),
			Type: event.Event, Value: event.Value, Style: event.Style,
			Anchor: event.Anchor, Tag: event.Tag,
			Flow: event.Flow, Explicit: event.Explicit,
		}
		handles := make([]string, 0, len(event.Tags))
		for handle := range event.Tags {
			handles = append(handles, handle)
		}
		sort.Strings(handles)
		for _, handle := range handles {
			events[i].TagDirectives = append(events[i].TagDirectives,
				yaml.TagDirective{Handle: handle, Prefix: event.Tags[handle]})
		}
		if event.Event == "alias" {
			events[i].Anchor = event.Name
		}
		if event.Version != "" {
			switch event.Version {
			case "1.1", "1.2":
				events[i].Version = &yaml.VersionDirective{
					Major: 1, Minor: int(event.Version[2] - '0'),
				}
			default:
				return nil, fmt.Errorf("unsupported YAML version %q", event.Version)
			}
		}
	}
	return events, nil
}
