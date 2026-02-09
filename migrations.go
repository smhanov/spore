package blog

// migration defines a single schema change for SQL-backed stores.
type migration struct {
	Version    int
	Name       string
	Statements []string
}

var migrations = []migration{
	{
		Version: 6,
		Name:    "create entities table",
		Statements: []string{
			SchemaBlogEntities,
		},
	},
}
