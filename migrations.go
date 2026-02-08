package blog

// migration defines a single schema change for SQL-backed stores.
type migration struct {
	Version    int
	Name       string
	Statements []string
}

var migrations = []migration{
	{
		Version: 1,
		Name:    "create blog tables",
		Statements: []string{
			SchemaBlogPosts,
			SchemaBlogTags,
			SchemaBlogPostTags,
		},
	},
	{
		Version: 2,
		Name:    "create ai settings table",
		Statements: []string{
			SchemaBlogAISettings,
		},
	},
}
