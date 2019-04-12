package migrate

const (
	UNKNOWN DataType = iota
	INT
	STRING
	FLOAT
	DOUBLE
	DATETIME
)

var typeToString map[DataType]string

func init() {
	typeToString = make(map[DataType]string)

	typeToString[UNKNOWN] = "unknown"
	typeToString[INT] = "int"
	typeToString[STRING] = "string"
	typeToString[FLOAT] = "float"
	typeToString[DOUBLE] = "double"
	typeToString[DATETIME] = "datetime"
}

func (t DataType) String() string {
	return typeToString[t]
}
