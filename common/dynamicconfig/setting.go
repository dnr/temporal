package dynamicconfig

type (
	Type int

	Precedence int

	Setting struct {
		// string value of key. case-insensitive.
		Key Key
		// type, for validation
		Type Type
		// precedence
		Precedence Precedence
		// default value. type must match Type, or else be a []ConstrainedValue where the
		// values match Type.
		Default any
		// documentation
		Description string
	}
)

const (
	TypeBool     Type = iota // go type: bool
	TypeInt                  // go type: int
	TypeFloat                // go type: float64
	TypeString               // go type: string
	TypeDuration             // go type: time.Duration
	TypeMap                  // go type: map[string]any
)

const (
	PrecedenceGlobal Precedence = iota
	PrecedenceNamespace
	PrecedenceNamespaceId
	PrecedenceTaskQueue
	PrecedenceShardId
	PrecedenceTaskType
)
