package config

// ShortLinkConfig short link configuration
// ShortLinkConfig 短链配置
type ShortLinkConfig struct {
	// Mode selects the short-link backend:
	//   "sink"  - third-party sink.cool (default, unchanged behaviour)
	//   "local" - built-in short links served by this instance at /s/<slug>
	// Mode 选择短链后端："sink" 为第三方 sink.cool（默认，行为不变）；
	// "local" 为内置短链，由本实例的 /s/<slug> 提供。
	Mode     string `yaml:"mode" default:"sink"`
	BaseURL  string `yaml:"base-url" default:"https://sink.cool"`
	APIKey   string `yaml:"api-key" default:"SinkCool"`
	Password string `yaml:"password" default:""`
	Cloaking bool   `yaml:"cloaking" default:"false"`
}
