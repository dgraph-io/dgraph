package dgraphlite

type Config struct {
	dataDir string
}

func (cc Config) WithDataDir(dir string) Config {
	cc.dataDir = dir
	return cc
}

// NewConfig generates a default Config for dgraphlite
func NewConfig() Config {
	return Config{}
}
