package gorocksdb

// CompressionOptions represents options for different compression algorithms like Zlib.
type CompressionOptions struct {
	WindowBits int
	Level      int
	Strategy   int
}

// NewDefaultCompressionOptions creates a default CompressionOptions object.
func NewDefaultCompressionOptions() *CompressionOptions {
	return NewCompressionOptions(-14, -1, 0)
}

// NewCompressionOptions creates a CompressionOptions object.
func NewCompressionOptions(windowBits, level, strategy int) *CompressionOptions {
	return &CompressionOptions{
		WindowBits: windowBits,
		Level:      level,
		Strategy:   strategy,
	}
}
