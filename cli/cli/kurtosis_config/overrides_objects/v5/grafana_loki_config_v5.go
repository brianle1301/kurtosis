package v5

type GrafanaLokiConfigV5 struct {
	// ShouldStartBeforeEngine starts Grafana and Loki before the engine, if true.
	// Equivalent to running `grafloki start` before `engine start`.
	// Useful for treating Grafana and Loki as default logging setup in Kurtosis.
	ShouldStartBeforeEngine bool   `yaml:"should-start-before-engine,omitempty"`
	GrafanaImage            string `yaml:"grafana-image,omitempty"`
	LokiImage               string `yaml:"loki-image,omitempty"`
}
