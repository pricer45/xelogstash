package config

import (
	"time"

	"github.com/billgraziano/toml"
)

// Source defines a source of extended event information
type Source struct {
	FQDN           string
	Sessions       []string
	IgnoreSessions bool `toml:"ignore_sessions"` // if true, skip XE sessions
	Prefix         string
	AgentJobs      string
	PayloadField   string `toml:"payload_field_name"`
	TimestampField string `toml:"timestamp_field_name"`
	Rows           int
	StripCRLF      bool      `toml:"strip_crlf"`
	StartAt        time.Time `toml:"start_at"`
	StopAt         time.Time `toml:"stop_at"`

	Adds               map[string]string
	Copies             map[string]string
	Moves              map[string]string
	ExcludedEvents     []string //XE events that are excluded.  Mostly from system health
	Exclude17830       bool     `toml:"exclude_17830"`
	LogBadXML          bool     `toml:"log_bad_xml"`
	IncludeDebugDLLMsg bool     `toml:"include_dbghelpdll_msg"`

	RawAdds   []string `toml:"adds"`
	RawCopies []string `toml:"copies"`
	RawMoves  []string `toml:"moves"`
}

// App defines the application configuration
type App struct {
	Workers  int
	Logstash string
	Samples  bool // Print sample JSON to stdout
	Summary  bool // Print a summary to stdout
	// Enables a web server on :8080 with basic metrics
	HTTPMetrics bool `toml:"http_metrics"`
}

// AppLog controls the application logging
type AppLog struct {
	Logstash       string
	PayloadField   string `toml:"payload_field_name"`
	TimestampField string `toml:"timestamp_field_name"`
	Samples        bool

	Adds   map[string]string
	Copies map[string]string
	Moves  map[string]string

	RawAdds   []string `toml:"adds"`
	RawCopies []string `toml:"copies"`
	RawMoves  []string `toml:"moves"`
}

// Config defines the configuration read from the TOML file
type Config struct {
	App      App
	AppLog   AppLog
	Defaults Source   `toml:"defaults"`
	Sources  []Source `toml:"source"`
	MetaData toml.MetaData
}
